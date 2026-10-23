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

package common

import (
	oracleErrors "github.com/oracle/go-oracledb/v26/oracle/errors"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// initMessagesIt initializes driver error messages in Italian.
func initMessagesIt() {
	translations := []struct {
		code    oracleErrors.ErrorCode
		message string
	}{
		{oracleErrors.InvalidCredential, "credenziale non valida o utente non autorizzato: %s"},
		{oracleErrors.ConnectionLost, "connessione al database chiusa dal peer"},
		{oracleErrors.AliasNotFound, "impossibile connettersi al database. Alias %s non trovato."},
		{oracleErrors.NoListenerAvailable, "impossibile connettersi. Nessun listener in %s"},
		{oracleErrors.ProtocolViolation, "violazione del protocollo"},
		{oracleErrors.ProtocolViolationLimitExceeded, "violazione del protocollo: limite di %s superato: limite [%d], valore [%d]"},
		{oracleErrors.LobClosed, "LOB già chiuso nella stessa transazione."},
		{oracleErrors.LobOpen, "LOB già aperto nella stessa transazione."},
		{oracleErrors.ChannelReadFailed, "fine del flusso rilevata. Il peer remoto ha chiuso la connessione."},
		{oracleErrors.ChannelWriteFailed, "scrittura sul canale non riuscita."},
		{oracleErrors.UnknownHost, "host specificato sconosciuto."},
		{oracleErrors.LogonDenied, "credenziale non valida o utente non autorizzato"},
		{oracleErrors.InvalidServiceName, "impossibile connettersi al database. Il servizio %s non è registrato presso il listener sull'host %s, porta %d. (CONNECTION_ID=%s)"},
		{oracleErrors.NoAvailableHandler, "impossibile connettersi al database. Nessun handler %s disponibile per il servizio richiesto %s sull'host %s, porta %d. (CONNECTION_ID=%s)"},
		{oracleErrors.UnknownInstance, "impossibile connettersi al database. L'istanza %s del servizio %s non è nota al listener sull'host %s, porta %d. (CONNECTION_ID=%s)"},
		{oracleErrors.UnresolvedAlias, "impossibile risolvere l'alias di connessione a causa di errori nella sintassi Easy Connect %s"},
		{oracleErrors.InvalidSID, "impossibile connettersi al database. Il SID %s non è registrato presso il listener sull'host %s, porta %d. (CONNECTION_ID=%s)"},
		{oracleErrors.ListenerRequestTimeout, "il listener non ha ricevuto la richiesta del client entro il tempo consentito"},
		{oracleErrors.ConnectionClosed, "connessione chiusa"},
		{oracleErrors.CtxTimeout, "timeout del contesto durante %s per %s (CONNECTION_ID=%s)"},
		{oracleErrors.ConnCloseTimeout, "si è verificato un timeout durante la chiusura della sessione di rete"},
		{oracleErrors.UnknownSPFFunction, "ricevuto codice di funzione sconosciuto dal server al client: %d"},
		{oracleErrors.SPFNotFunction, "il messaggio non implementa l'interfaccia Function"},
		{oracleErrors.InvalidSPFFunction, "il messaggio non è del tipo previsto: [%s]"},
		{oracleErrors.InvalidConnectionParameter, "valore non valido [%s] per il parametro di connessione [%s]; valori accettati: [%s]"},
		{oracleErrors.ConflictingConnectionParameterSource, "il parametro [%s] non può essere specificato in più origini"},
		{oracleErrors.NamingDSNInvalid, "impossibile analizzare %s dal DSN"},
		{oracleErrors.NamingParseFailed, "stringa di connessione non valida"},
		{oracleErrors.NamingParsePosition, "nome previsto dopo '(' alla posizione %d"},
		{oracleErrors.NamingParseValue, "token non valido %q nella stringa di connessione"},
		{oracleErrors.NamingParseValues, "stringa di connessione non valida: radice ottenuta %q, prevista %q"},
		{oracleErrors.NamingParsePath, "percorso non valido %q nella stringa di connessione"},
		{oracleErrors.NamingParsePathSegment, "segmento di percorso non valido %q in %q alla posizione %d"},
		{oracleErrors.NamingParseBounds, "limiti non validi nella stringa di connessione: ottenuto %d, previsto %d"},
		{oracleErrors.NamingEzConnectError, "valore non valido %q per %s nella stringa di connessione"},
		{oracleErrors.NamingContextError, "valore non valido %q per %s nel descrittore di connessione"},
		{oracleErrors.FailMarshal, "serializzazione del messaggio non riuscita: %s"},
		{oracleErrors.FailUnmarshal, "deserializzazione del messaggio non riuscita: %s"},
		{oracleErrors.InvalidLOBBuffer, "configurazione del buffer LOB non valida durante %s (%s): %s"},
		{oracleErrors.LobExecError, "operazione LOB non riuscita durante %s."},
		{oracleErrors.UnsupportedCharacterSet, "set di caratteri non supportato: %d"},
		{oracleErrors.InvalidNetworkValue, "valore non valido per %s: %v"},
		{oracleErrors.InvalidNetworkExpectedValue, "valore non valido per %s: ottenuto %v, previsto %v"},
		{oracleErrors.InvalidNetworkContextValue, "valore non valido per %s (%s): %v"},
		{oracleErrors.InvalidNetworkContextExpectedValue, "valore non valido per %s (%s): ottenuto %v, previsto %v"},
		{oracleErrors.InvalidNetworkLength, "lunghezza non valida per %s: %d"},
		{oracleErrors.InvalidNetworkExpectedLength, "lunghezza non valida per %s: ottenuta %d, prevista %d"},
		{oracleErrors.InvalidNetworkContextExpectedLength, "lunghezza non valida per %s (%s): ottenuta %d, prevista %d"},
		{oracleErrors.ErrConnectionInband, "errore ricevuto dal server in-band"},
		{oracleErrors.NetworkDataReadFailed, "lettura dei dati di rete non riuscita"},
		{oracleErrors.ConnectionRefusedDetail, "connessione rifiutata: ORA-%s, motivo utente %d, motivo di sistema %d"},
		{oracleErrors.NetworkServerErrorCode, "%s-%s"},
		{oracleErrors.BreakPacketReceived, "ricevuto pacchetto di interruzione"},
		{oracleErrors.RefuseDataParseFailed, "analisi dei dati di rifiuto non riuscita"},
		{oracleErrors.RedirectAddressMissing, "i dati di reindirizzamento non contengono alcun indirizzo"},
		{oracleErrors.TLSRenegotiationUnsupported, "questa connessione non supporta la rinegoziazione TLS"},
		{oracleErrors.UnexpectedConnectResponse, "pacchetto imprevisto durante la connessione"},
		{oracleErrors.NamingInputMissing, "manca %s"},
		{oracleErrors.NamingTokensMissing, "la stringa di connessione non contiene token validi"},
		{oracleErrors.NamingUnexpectedClosingParenthesis, "parentesi di chiusura ')' imprevista alla posizione %d"},
		{oracleErrors.NetworkRetryLimitExceeded, "troppi %s: limite massimo di %d superato"},
		{oracleErrors.InvalidNetworkProperty, "valore non valido per la proprietà %s"},
		{oracleErrors.PKCS8ParseFailed, "analisi della chiave privata crittografata PKCS#8 non riuscita"},
		{oracleErrors.PKCS8DecryptFailed, "decrittografia della chiave privata crittografata PKCS#8 non riuscita"},
		{oracleErrors.PKCS7PaddingInvalid, "padding PKCS#7 non valido"},
		{oracleErrors.TLSCertificateVerificationFailed, "verifica del certificato del server non riuscita"},
		{oracleErrors.TLSCertificateDNMatchFailed, "il DN del certificato del server non corrisponde"},
		{oracleErrors.PEMBlockDecryptFailed, "decrittografia del blocco PEM non riuscita"},
		{oracleErrors.WalletKeyPairLoadFailed, "caricamento della coppia di chiavi del wallet non riuscito"},
		{oracleErrors.ConfiguredDNInvalid, "DN configurato non valido"},
		{oracleErrors.CertificateSubjectInvalid, "subject del certificato del server non valido"},
		{oracleErrors.PKCS8ParametersParseFailed, "analisi dei parametri %s non riuscita"},
		{oracleErrors.PKCS8InitializationVectorParseFailed, "analisi del vettore di inizializzazione PKCS#8 non riuscita"},
		{oracleErrors.DNMalformedAttribute, "attributo DN malformato %s"},
		{oracleErrors.DNUnsupportedAttribute, "attributo DN non supportato %s"},
		{oracleErrors.DNAttributeValueMissing, "manca il valore dell'attributo DN %s"},
		{oracleErrors.DNMalformedEscape, "sequenza di escape DN malformata"},
		{oracleErrors.DNMismatchAtRDN, "DN non corrispondente a RDN %d"},
		{oracleErrors.DNAttributeMissingFromCertificate, "attributo DN configurato con OID %s non trovato nel subject del certificato del server"},
		{oracleErrors.DNUnsupportedAttributeOID, "il DN configurato contiene un OID di attributo non supportato %s"},
		{oracleErrors.DNDuplicateAttributeOID, "il DN configurato contiene un OID di attributo duplicato %s"},
		{oracleErrors.DNAttributeOIDValueTypeInvalid, "il valore dell'attributo DN con OID %s non è una stringa"},
		{oracleErrors.DNHexValueUnsupported, "i valori dell'attributo DN codificati in esadecimale non sono supportati"},
		{oracleErrors.DNAttributeValueInvalidUTF8, "il valore dell'attributo DN non è UTF-8 valido"},
		{oracleErrors.CertificateSubjectTrailingData, "il subject del certificato del server contiene dati residui"},
		{oracleErrors.CertificateSubjectParseFailed, "analisi del subject del certificato del server non riuscita"},
		{oracleErrors.WalletCACertificatesMissing, "il wallet non contiene certificati CA"},
		{oracleErrors.WalletCACertificatesParseFailed, "analisi dei certificati CA del wallet non riuscita"},
		{oracleErrors.NetworkCompressionFailed, "compressione del pacchetto di rete con %s non riuscita"},
		{oracleErrors.NetworkDecompressionFailed, "decompressione del pacchetto di rete con %s non riuscita"},
		{oracleErrors.ConnectTimeout, "%s: timeout di %d scaduto per %s. (CONNECTION_ID=%s)"},
		{oracleErrors.ListenerFailed, "TNS: il listener non è riuscito ad avviare un processo server dedicato"},
		{oracleErrors.NoDispatcherAvailable, "TNS: il listener non ha trovato un handler disponibile con lo stack di protocolli corrispondente"},
		{oracleErrors.ConnectFailed, "connessione non riuscita"},
		{oracleErrors.TableOrViewNotFound, "la tabella o la vista non esiste"},
		{oracleErrors.InsufficientPrivilege, "privilegi insufficienti"},
		{oracleErrors.InvalidTableName, "nome di tabella non valido"},
		{oracleErrors.MissingReadPrivilege, "privilegio READ mancante"},
		{oracleErrors.ConverterEmptyInput, "convertitore %s %s: input vuoto"},
		{oracleErrors.ConverterRange, "convertitore %s %s: %s; previsto=%v..%v"},
		{oracleErrors.ConverterExpectedFormat, "convertitore %s %s: %s; formato previsto=%v"},
		{oracleErrors.RowDecodeError, "decodifica di %s per la colonna %s (indice %d) non riuscita"},
		{oracleErrors.RowsCloseFailed, "chiusura delle righe dell'istruzione non riuscita: %s"},
		{oracleErrors.StatementExecutionFailed, "preparazione dell'istruzione %s non riuscita: %s."},
		{oracleErrors.StatementExecutionFactoryFailed, "impossibile ottenere Executor dalla factory; tipo non valido: %s per %s"},
		{oracleErrors.StatementCloseFailed, "chiusura dell'istruzione non riuscita: %s"},
		{oracleErrors.RunQueryError, "runQuery non riuscito: %s."},
		{oracleErrors.RunExecError, "runExec non riuscito: %s."},
		{oracleErrors.CallbackFactoryError, "factory di messaggi non riuscita: %s."},
		{oracleErrors.StatementParsingDanglingColon, "due punti senza identificatore alla fine dell'SQL"},
		{oracleErrors.StatementParsingPlaceholdersArgsMismatch, "trovati %d segnaposto, ma forniti %d argomenti"},
		{oracleErrors.StatementParsingInvalidArgCount, "numero di argomenti non valido: forniti %d, attesi %d segnaposto univoci"},
		{oracleErrors.StatementParsingNameNotFound, "nome del segnaposto %q non trovato nell'SQL"},
		{oracleErrors.StatementParsingDuplicateArg, "valore duplicato per il segnaposto %q all'occorrenza numero %d"},
		{oracleErrors.StatementParsingInvalidOrdinal, "ordinale %d non valido; trovati %d segnaposto"},
		{oracleErrors.StatementParsingMissingValue, "valore mancante per il segnaposto %q all'occorrenza numero %d"},
		{oracleErrors.InvalidIdentifier, "identificatore SQL non valido"},
		{oracleErrors.CancelOperationError, "annullamento dell'operazione non riuscito"},
		{oracleErrors.InternalError, "si è verificato un errore interno."},
		{oracleErrors.NetworkInternalError, "errore di rete interno: %s"},
		{oracleErrors.MissingLocalizationService, "servizio di localizzazione mancante in shelf"},
		{oracleErrors.UnsupportedFeature, "funzionalità non supportata dal driver: %s"},
		{oracleErrors.HTTPSProxyRequiresTCPS, "il proxy HTTPS richiede una connessione TCPS"},
		{oracleErrors.HTTPSProxyConnectFailed, "richiesta CONNECT del proxy HTTPS non riuscita per %s"},
		{oracleErrors.InvalidSqlOutParameter, "parametro sql.Out non valido: %s"},
		{oracleErrors.InvalidFileConfig, "caricamento della configurazione non riuscito"},
		{oracleErrors.NotInTransaction, "nessuna transazione attiva"},
		{oracleErrors.IsolationLevelNotSupported, "livello di isolamento non supportato"},
		{oracleErrors.AlreadyInTransaction, "transazione già avviata"},
		{oracleErrors.ErrorInTransaction, "si è verificato un errore durante l'operazione di transazione %s"},
		{oracleErrors.ConfigureTransactionError, "configurazione della transazione non riuscita"},
		{oracleErrors.AuthenticatorError, "si è verificato un errore durante l'autenticazione"},
		{oracleErrors.NegotiatorError, "si è verificato un errore durante la negoziazione della connessione"},
		{oracleErrors.MarshalEngineError, "si è verificato un errore durante la serializzazione o deserializzazione di un valore di tipo %s"},
		{oracleErrors.MarshalEngineFlushError, "si è verificato un errore durante lo svuotamento dei dati"},
		{oracleErrors.StreamerReadError, "si è verificato un errore durante la lettura di un messaggio"},
		{oracleErrors.StreamerWriteError, "si è verificato un errore durante la scrittura di un messaggio"},
		{oracleErrors.DatabaseMountStateError, "il database è nello stato NOMOUNT"},
		{oracleErrors.EmptyUsernameError, "i nomi utente vuoti non sono supportati"},
		{oracleErrors.NoAuthenticatorError, "nessun autenticatore disponibile per i parametri forniti"},
		{oracleErrors.ServerTimeZoneError, "operazione %s sul fuso orario del server non riuscita"},
		{oracleErrors.OsonUnsupportedScalarError, "il documento OSON contiene un codice operativo scalare non supportato 0x%02x"},
		{oracleErrors.JSONAccessError, "accesso JSON non valido: %s"},
		{oracleErrors.JSONUninitializedValue, "il metodo JSON %s è stato chiamato su un valore non inizializzato"},
		{oracleErrors.JSONScanTypeUnsupportedError, "JSON non può leggere un valore di tipo %s"},
		{oracleErrors.JSONArrayIndexOutOfRangeError, "l'indice dell'array JSON %d è fuori intervallo"},
		{oracleErrors.OsonEncodingError, "impossibile codificare il valore come documento OSON"},
		{oracleErrors.JSONRenderingError, "impossibile rappresentare il valore JSON Oracle come testo JSON"},
		{oracleErrors.JSONObjectKeyNotFoundError, "chiave dell'oggetto JSON non trovata"},
		{oracleErrors.InvalidConnection, "la connessione fornita non è una connessione Oracle"},
		{oracleErrors.EmptyTokenError, "l'ID del token per l'autenticazione basata su token è vuoto"},
		{oracleErrors.TokenAuthenticationError, "si è verificato un errore durante l'autenticazione tramite token"},
		{oracleErrors.ValueRetrievalError, "recupero del valore %s non riuscito"},
		{oracleErrors.InvalidSignedTokenPrivateKey, "chiave privata del token firmato non valida"},
		{oracleErrors.ExpiredToken, "il token di accesso è scaduto"},
		{oracleErrors.ProviderNotFound, "nessun provider del tipo richiesto trovato"},
	}

	for _, translation := range translations {
		message.SetString(language.Italian, string(translation.code), translation.message)
	}
}
