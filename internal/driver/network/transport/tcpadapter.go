/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package transport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/oracle/go-oracledb/v26/internal/common"
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
)

const (
	DEFAULT_HTTPS_PROXY_PORT = 80
	TCPCHA                   = 1<<1 | 1<<2 | 1<<3 | 1<<8 | 1<<9 | 1<<12
)

func httpsProxyPortOrDefault(port int) int {
	if port == 0 {
		return DEFAULT_HTTPS_PROXY_PORT
	}
	return port
}

// nttcp represents a TCP network transport adapter
type nttcp struct {
	cha        int
	connected  bool
	secure     bool
	host       string
	hostname   string
	originHost string
	port       uint16
	stream     net.Conn
	atts       NTattributes
	_writerWG  sync.WaitGroup
	_readerWG  sync.WaitGroup
}

func NewNTTCP(atts NTattributes, port uint16) *nttcp {
	return &nttcp{
		cha:       TCPCHA,
		connected: false,
		secure:    false,
		atts:      atts,
		port:      port,
	}
}

// RemoteAddr returns the remote address of the underlying stream, or nil when
// no stream is connected.
func (nt *nttcp) RemoteAddr() net.Addr {
	if nt.stream == nil {
		return nil
	}
	return nt.stream.RemoteAddr()
}

type ioOperationResult struct {
	byteCount int   // number of bytes read or write
	ioerr     error // error of the I/O operation
}

// Send writes all bytes in buf to the underlying stream.
// ctx controls the write operation and buf contains the bytes to send.
// It returns an error when the write fails or the context is cancelled.
func (nt *nttcp) Send(ctx context.Context, buf []byte) error {

	var ctxToBeUsed context.Context
	var cancel context.CancelFunc
	ctxToBeUsed = ctx
	if nt.atts.SendTimeout > 0 {
		ctxToBeUsed, cancel = context.WithTimeout(ctx, time.Duration(nt.atts.SendTimeout)*time.Millisecond)
		defer cancel()
	}

	resCh := make(chan ioOperationResult, 1)

	nt._writerWG.Go(func() {
		bytesWritten := 0
		bufLen := len(buf)
		for bytesWritten < bufLen {
			chunk := buf[bytesWritten:]
			n, err := nt.stream.Write(chunk)
			if err != nil {
				// timeout is not handle here (it cannot be raised in this routine), as it is handled in the main thread.
				resCh <- ioOperationResult{byteCount: bytesWritten, ioerr: err}
				return
			}
			bytesWritten += n
		}
		resCh <- ioOperationResult{byteCount: bytesWritten, ioerr: nil}
	})

	select {
	case res := <-resCh:
		if res.ioerr != nil {
			return common.NewOracleError(oracleErrors.ChannelWriteFailed, res.ioerr)
		}
		return nil
	case <-ctxToBeUsed.Done():
		// kick out go routine stuck on read now.
		common.Odl.Debug("context for Send() cancelled, cancelling writer")
		nt.stream.SetWriteDeadline(time.Now())
		nt._writerWG.Wait()
		common.Odl.Debug("Send() timed-out, writer is now joined")
		// reset the deadline
		nt.stream.SetWriteDeadline(time.Time{})
		return common.NewOracleError(oracleErrors.CtxTimeout, ctx.Err(), "send", fmt.Sprintf("%s:%d", nt.host, nt.port), nt.atts.Connectionid)
	}
}

// Receive reads bytes2Read bytes from the underlying stream into buf.
// ctx controls the read, buf receives the bytes, and bytes2Read is the number
// of bytes requested. It returns the number of bytes read and an error when
// buf is too small, the stream read fails, or the context is cancelled.
func (nt *nttcp) Receive(ctx context.Context, buf []byte, bytes2Read int) (int, error) {

	if bytes2Read > len(buf) {
		return 0, common.NewOracleError(oracleErrors.InvalidNetworkExpectedLength, nil, "buffer", len(buf), bytes2Read)
	}

	var ctxToBeUsed context.Context
	var cancel context.CancelFunc
	ctxToBeUsed = ctx
	if nt.atts.RecvTimeout > 0 {
		ctxToBeUsed, cancel = context.WithTimeoutCause(ctx, time.Duration(nt.atts.RecvTimeout)*time.Millisecond,
			common.NewCtxTimeoutCauseError("recv-timeout", uint(nt.atts.RecvTimeout),
				nt.atts.Connectionid))
		defer cancel()
	}

	resCh := make(chan ioOperationResult, 1)

	nt._readerWG.Go(func() {
		bytesRead := 0
		for bytesRead < bytes2Read {
			remainder := bytes2Read - bytesRead
			segment := buf[bytesRead : bytesRead+remainder]
			n, err := nt.stream.Read(segment)
			if err != nil {
				// timeout is not handle here (it cannot be raised in this routine), as it is handled in the main thread.
				resCh <- ioOperationResult{byteCount: bytesRead, ioerr: err}
				return
			}
			bytesRead += n
		}
		resCh <- ioOperationResult{byteCount: bytesRead, ioerr: nil}
	})

	select {
	case res := <-resCh:
		if res.ioerr != nil {
			return res.byteCount, common.NewOracleError(oracleErrors.ChannelReadFailed, res.ioerr)
		}
		return res.byteCount, nil
	case <-ctxToBeUsed.Done():
		// kick out go routine stuck on read now.
		common.Odl.Debug("context for Receive() cancelled, cancelling reader")
		nt.stream.SetReadDeadline(time.Now())
		nt._readerWG.Wait()
		common.Odl.Debug("Receive() timed-out, reader is now joined")
		// reset the deadline
		nt.stream.SetReadDeadline(time.Time{})
		return 0, context.Cause(ctxToBeUsed)
	}
}

// nTConnect establishes a TCP connection
func (nt *nttcp) nTConnect(ctx context.Context, address Address) error {

	targetHost := address.Hostname
	if targetHost == "" {
		targetHost = address.Host
	}
	target := net.JoinHostPort(targetHost, strconv.Itoa(int(address.Port)))
	var httpsProxy string
	var httpsProxyPort int
	if address.HTTPSProxy != "" {
		httpsProxy = address.HTTPSProxy
		httpsProxyPort = address.HTTPSProxyPort
	} else {
		httpsProxy = nt.atts.HttpsProxy
		httpsProxyPort = nt.atts.HttpsProxyPort
	}
	var dialer net.Dialer

	var dialCtxToBeUsed context.Context
	var dialCancelToBeUsed context.CancelFunc
	dialCtxToBeUsed = ctx
	if nt.atts.Transportconnecttimeout > 0 {
		// take precedence over the passed context if it is a shorter timeout
		dialCtxToBeUsed, dialCancelToBeUsed =
			context.WithTimeoutCause(ctx,
				time.Duration(nt.atts.Transportconnecttimeout)*time.Millisecond,
				common.NewCtxTimeoutCauseError("TransportConnectTimeout",
					uint(nt.atts.Transportconnecttimeout),
					nt.atts.Connectionid))
		defer dialCancelToBeUsed()
	}
	dialAddress := address.String()
	if httpsProxy != "" {
		httpsProxyPort = httpsProxyPortOrDefault(httpsProxyPort)
		dialAddress = net.JoinHostPort(httpsProxy, strconv.Itoa(httpsProxyPort))
	}
	common.Odl.Debug("dialing remote host")
	conn, err := dialer.DialContext(dialCtxToBeUsed, "tcp", dialAddress)
	if err != nil {
		if httpsProxy == "" {
			return normalizeDialError(dialCtxToBeUsed, err, address, nt.atts.Connectionid)
		}
		proxyAddress := address
		proxyAddress.Host = httpsProxy
		proxyAddress.Port = uint16(httpsProxyPort)
		return normalizeDialError(dialCtxToBeUsed, err, proxyAddress, nt.atts.Connectionid)
	}
	if httpsProxy != "" {
		request, reqErr := http.NewRequestWithContext(dialCtxToBeUsed, http.MethodConnect, "http://"+target, nil)
		if reqErr != nil {
			_ = conn.Close()
			return common.NewOracleError(oracleErrors.HTTPSProxyConnectFailed, reqErr, target)
		}
		request.Host = target
		reader := bufio.NewReader(conn)
		type proxyHandshakeResult struct {
			response *http.Response
			err      error
		}
		resultCh := make(chan proxyHandshakeResult, 1)
		go func() {
			if err := request.Write(conn); err != nil {
				resultCh <- proxyHandshakeResult{err: err}
				return
			}
			response, err := http.ReadResponse(reader, request)
			resultCh <- proxyHandshakeResult{response: response, err: err}
		}()

		var response *http.Response
		select {
		case result := <-resultCh:
			response = result.response
			if result.err != nil {
				_ = conn.Close()
				return common.NewOracleError(oracleErrors.HTTPSProxyConnectFailed, result.err, target)
			}
		case <-dialCtxToBeUsed.Done():
			// Interrupt a blocked Request.Write or ReadResponse, then wait for
			// the goroutine before closing the connection.
			_ = conn.SetDeadline(time.Now())
			<-resultCh
			_ = conn.Close()
			if timeoutCause, ok := context.Cause(dialCtxToBeUsed).(common.CtxTimeoutCauseError); ok {
				return timeoutCause
			}
			return common.NewOracleError(oracleErrors.HTTPSProxyConnectFailed,
				context.Cause(dialCtxToBeUsed), target)
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			_ = response.Body.Close()
			_ = conn.Close()
			return common.NewOracleError(oracleErrors.HTTPSProxyConnectFailed, errors.New(response.Status), target)
		}
		_ = response.Body.Close()
	}
	nt.stream = conn
	nt.connected = true
	return nil
}

// normalizeDialError preserves DNS errors so callers can distinguish a failed
// hostname lookup from a failed TCP connection to a resolved endpoint.
func normalizeDialError(ctx context.Context, err error, address Address, connectionID string) error {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return err
	}

	var opError *net.OpError
	if !errors.As(err, &opError) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) || opError.Timeout() {
		reportedCause := context.Cause(ctx)
		if sqlE, ok := reportedCause.(oracleErrors.SQLError); ok {
			return sqlE
		}
		if te, ok := reportedCause.(common.CtxTimeoutCauseError); ok {
			return te
		}
		// deal with a context case now as we always want an oracleErrors
		return common.NewOracleError(oracleErrors.CtxTimeout, nil, "CONNECT",
			address.String(), connectionID)
	}
	if opError.Op == "dial" && errors.Is(opError.Err, syscall.ECONNREFUSED) {
		return common.NewOracleError(oracleErrors.NoListenerAvailable, nil, address.String())
	}
	return err
}

// Connect establishes a network transport connection to address.
func (nt *nttcp) Connect(ctx context.Context, address Address) error {
	nt.originHost = address.OriginHost
	nt.host = address.Host
	nt.hostname = address.Hostname
	if nt.hostname == "" {
		nt.hostname = address.Host
	}
	nt.port = address.Port

	if err := nt.nTConnect(ctx, address); err != nil {
		return err
	}
	cleanupOnError := func(err error) error {
		if disconnectErr := nt.Disconnect(); disconnectErr != nil {
			return errors.Join(err, disconnectErr)
		}
		return err
	}
	if nt.atts.ExpireTime > 0 || nt.atts.EnabledDCD {
		tcpConn, ok := nt.stream.(*net.TCPConn)
		if ok {
			if err := tcpConn.SetKeepAlive(true); err != nil {
				return cleanupOnError(err)
			}
			if nt.atts.ExpireTime > 0 {
				if err := tcpConn.SetKeepAlivePeriod(time.Duration(nt.atts.ExpireTime) * time.Millisecond); err != nil {
					return cleanupOnError(err)
				}
			}
		}
	}
	if !nt.atts.TCPNODelay {
		if tcpConn, ok := nt.stream.(*net.TCPConn); ok {
			if err := tcpConn.SetNoDelay(false); err != nil {
				return cleanupOnError(err)
			}
		}
	}
	return nil
}

// Disconnect closes the transport stream and clears the connected state.
func (nt *nttcp) Disconnect() error {
	nt.connected = false
	if nt.stream != nil {
		err := nt.stream.Close()
		nt.stream = nil
		return err
	}
	return nil
}
