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
	"context"
	"errors"
	"fmt"
	"net"
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

type ioOperationResult struct {
	byteCount int   // number of bytes read or write
	ioerr     error // error of the I/O operation
}

// Send send buffer bytes to the underlying stream
// parameters:
//   - ctx : context to be used while reading
//   - buf : the buffer to be sent.
//
// returns :
//   - error raised during read operation
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

// Receive read bytes2Read byte form underlying stream
// parameters:
//   - ctx : context to be used while reading
//   - buf : the buffer to copy bytes to.
//   - bytes2Read: number of byte sot be read
//
// returns :
//   - read bytes count
//   - error raised during read operation
func (nt *nttcp) Receive(ctx context.Context, buf []byte, bytes2Read int) (int, error) {

	if bytes2Read > len(buf) {
		return 0, fmt.Errorf("buffer too small: have %d want %d", len(buf), bytes2Read)
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

	var httpsProxy string
	var httpsProxyPort int
	if address.HTTPSProxy != "" {
		httpsProxy = address.HTTPSProxy
		httpsProxyPort = address.HTTPSProxyPort
	} else {
		httpsProxy = nt.atts.HttpsProxy
		httpsProxyPort = nt.atts.HttpsProxyPort
	}
	if httpsProxyPort == 0 {
		httpsProxyPort = DEFAULT_HTTPS_PROXY_PORT
	}

	if httpsProxy != "" {
		return fmt.Errorf("HTTPS proxy support is not implemented")
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
	common.Odl.Debug("dialing remote host")
	conn, err := dialer.DialContext(dialCtxToBeUsed, "tcp", address.String())
	if err != nil {
		opError := err.(*net.OpError)
		if errors.Is(err, context.DeadlineExceeded) ||
			opError.Timeout() {
			reportedCause := context.Cause(dialCtxToBeUsed)
			if sqlE, ok := reportedCause.(oracleErrors.SQLError); ok {
				return sqlE
			}
			if te, ok := reportedCause.(common.CtxTimeoutCauseError); ok {
				return te
			}
			// deal with a context case now as we always want an oracleErrors
			return common.NewOracleError(oracleErrors.CtxTimeout, nil, "CONNECT",
				address.String(), nt.atts.Connectionid)
		}
		if opError.Op == "dial" && errors.Is(opError.Err, syscall.ECONNREFUSED) {
			return common.NewOracleError(oracleErrors.NoListenerAvailable, nil, address.String())
		}
		return err
	}
	nt.stream = conn
	nt.connected = true
	return nil
}

// Connect establishes a network transport connection
func (nt *nttcp) Connect(ctx context.Context, address Address) error {
	nt.originHost = address.OriginHost
	nt.host = address.Host
	nt.hostname = address.Hostname
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

func (nt *nttcp) Disconnect() error {
	nt.connected = false
	if nt.stream != nil {
		err := nt.stream.Close()
		nt.stream = nil
		return err
	}
	return nil
}
