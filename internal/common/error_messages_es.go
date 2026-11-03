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

// initMessagesEs initializes driver error messages in Spanish.
func initMessagesEs() {
	translations := []struct {
		code    oracleErrors.ErrorCode
		message string
	}{
		{oracleErrors.InvalidCredential, "credenciales no válidas o usuario no autorizado: %s"},
		{oracleErrors.ConnectionLost, "conexión a la base de datos cerrada por el extremo remoto"},
		{oracleErrors.AliasNotFound, "no se puede conectar a la base de datos. No se encontró el alias %s."},
		{oracleErrors.NoListenerAvailable, "no se puede conectar. No hay ningún listener en %s"},
		{oracleErrors.ProtocolViolation, "infracción del protocolo"},
		{oracleErrors.ProtocolViolationLimitExceeded, "infracción del protocolo: se superó el límite de %s: límite [%d], valor [%d]"},
		{oracleErrors.LobClosed, "el LOB ya está cerrado en la misma transacción."},
		{oracleErrors.LobOpen, "el LOB ya está abierto en la misma transacción."},
		{oracleErrors.ChannelReadFailed, "se detectó el fin del flujo. El extremo remoto cerró la conexión."},
		{oracleErrors.ChannelWriteFailed, "no se pudo escribir en el canal."},
		{oracleErrors.UnknownHost, "se especificó un host desconocido."},
		{oracleErrors.LogonDenied, "credenciales no válidas o usuario no autorizado"},
		{oracleErrors.InvalidServiceName, "no se puede conectar a la base de datos. El servicio %s no está registrado en el listener del host %s, puerto %d. (CONNECTION_ID=%s)"},
		{oracleErrors.NoAvailableHandler, "no se puede conectar a la base de datos. No hay ningún manejador %s disponible para el servicio solicitado %s en el host %s, puerto %d. (CONNECTION_ID=%s)"},
		{oracleErrors.UnknownInstance, "no se puede conectar a la base de datos. El listener no conoce la instancia %s del servicio %s en el host %s, puerto %d. (CONNECTION_ID=%s)"},
		{oracleErrors.UnresolvedAlias, "no se puede resolver el alias de conexión debido a errores en la sintaxis de Easy Connect %s"},
		{oracleErrors.InvalidSID, "no se puede conectar a la base de datos. El SID %s no está registrado en el listener del host %s, puerto %d. (CONNECTION_ID=%s)"},
		{oracleErrors.ListenerRequestTimeout, "el listener no recibió la solicitud del cliente dentro del tiempo permitido"},
		{oracleErrors.ConnectionClosed, "conexión cerrada"},
		{oracleErrors.CtxTimeout, "el contexto agotó el tiempo de espera durante %s para %s (CONNECTION_ID=%s)"},
		{oracleErrors.ConnCloseTimeout, "se agotó el tiempo de espera al cerrar la sesión de red"},
		{oracleErrors.UnknownSPFFunction, "se recibió un código de función desconocido del servidor al cliente: %d"},
		{oracleErrors.SPFNotFunction, "el mensaje no implementa la interfaz Function"},
		{oracleErrors.InvalidSPFFunction, "el mensaje no es del tipo esperado: [%s]"},
		{oracleErrors.InvalidConnectionParameter, "valor no válido [%s] para el parámetro de conexión [%s]; valores aceptados: [%s]"},
		{oracleErrors.ConflictingConnectionParameterSource, "el parámetro [%s] no se puede especificar en más de una fuente"},
		{oracleErrors.NamingDSNInvalid, "no se puede analizar %s del DSN"},
		{oracleErrors.NamingParseFailed, "cadena de conexión no válida"},
		{oracleErrors.NamingParsePosition, "se esperaba un nombre después de '(' en la posición %d"},
		{oracleErrors.NamingParseValue, "token no válido %q en la cadena de conexión"},
		{oracleErrors.NamingParseValues, "cadena de conexión no válida: se obtuvo la raíz %q, se esperaba %q"},
		{oracleErrors.NamingParsePath, "ruta no válida %q en la cadena de conexión"},
		{oracleErrors.NamingParsePathSegment, "segmento de ruta no válido %q en %q en la posición %d"},
		{oracleErrors.NamingParseBounds, "límites no válidos en la cadena de conexión: se obtuvo %d, se esperaba %d"},
		{oracleErrors.NamingEzConnectError, "valor no válido %q para %s en la cadena de conexión"},
		{oracleErrors.NamingContextError, "valor no válido %q para %s en el descriptor de conexión"},
		{oracleErrors.FailMarshal, "no se pudo serializar el mensaje: %s"},
		{oracleErrors.FailUnmarshal, "no se pudo deserializar el mensaje: %s"},
		{oracleErrors.InvalidLOBBuffer, "configuración no válida del búfer LOB durante %s (%s): %s"},
		{oracleErrors.LobExecError, "no se pudo realizar la operación LOB durante %s."},
		{oracleErrors.UnsupportedCharacterSet, "conjunto de caracteres no compatible: %d"},
		{oracleErrors.InvalidNetworkValue, "valor no válido para %s: %v"},
		{oracleErrors.InvalidNetworkExpectedValue, "valor no válido para %s: se obtuvo %v, se esperaba %v"},
		{oracleErrors.InvalidNetworkContextValue, "valor no válido para %s (%s): %v"},
		{oracleErrors.InvalidNetworkContextExpectedValue, "valor no válido para %s (%s): se obtuvo %v, se esperaba %v"},
		{oracleErrors.InvalidNetworkLength, "longitud no válida para %s: %d"},
		{oracleErrors.InvalidNetworkExpectedLength, "longitud no válida para %s: se obtuvo %d, se esperaba %d"},
		{oracleErrors.InvalidNetworkContextExpectedLength, "longitud no válida para %s (%s): se obtuvo %d, se esperaba %d"},
		{oracleErrors.ErrConnectionInband, "se recibió un error del servidor en banda"},
		{oracleErrors.NetworkDataReadFailed, "no se pudieron leer los datos de red"},
		{oracleErrors.ConnectionRefusedDetail, "conexión rechazada: ORA-%s, motivo del usuario %d, motivo del sistema %d"},
		{oracleErrors.NetworkServerErrorCode, "%s-%s"},
		{oracleErrors.BreakPacketReceived, "se recibió un paquete de interrupción"},
		{oracleErrors.RefuseDataParseFailed, "no se pudieron analizar los datos de rechazo"},
		{oracleErrors.RedirectAddressMissing, "los datos de redirección no contienen ninguna dirección"},
		{oracleErrors.TLSRenegotiationUnsupported, "esta conexión no admite la renegociación de TLS"},
		{oracleErrors.UnexpectedConnectResponse, "paquete inesperado durante la conexión"},
		{oracleErrors.NamingInputMissing, "falta %s"},
		{oracleErrors.NamingTokensMissing, "la cadena de conexión no contiene tokens válidos"},
		{oracleErrors.NamingUnexpectedClosingParenthesis, "paréntesis de cierre ')' inesperado en la posición %d"},
		{oracleErrors.NetworkRetryLimitExceeded, "demasiados %s: se superó el máximo de %d"},
		{oracleErrors.InvalidNetworkProperty, "valor no válido para la propiedad %s"},
		{oracleErrors.PKCS8ParseFailed, "no se pudo analizar la clave privada cifrada PKCS#8"},
		{oracleErrors.PKCS8DecryptFailed, "no se pudo descifrar la clave privada cifrada PKCS#8"},
		{oracleErrors.PKCS7PaddingInvalid, "relleno PKCS#7 no válido"},
		{oracleErrors.TLSCertificateVerificationFailed, "no se pudo verificar el certificado del servidor"},
		{oracleErrors.TLSCertificateDNMatchFailed, "el DN del certificado del servidor no coincide"},
		{oracleErrors.PEMBlockDecryptFailed, "no se pudo descifrar el bloque PEM"},
		{oracleErrors.WalletKeyPairLoadFailed, "no se pudo cargar el par de claves del wallet"},
		{oracleErrors.ConfiguredDNInvalid, "DN configurado no válido"},
		{oracleErrors.CertificateSubjectInvalid, "asunto del certificado del servidor no válido"},
		{oracleErrors.PKCS8ParametersParseFailed, "no se pudieron analizar los parámetros %s"},
		{oracleErrors.PKCS8InitializationVectorParseFailed, "no se pudo analizar el vector de inicialización PKCS#8"},
		{oracleErrors.DNMalformedAttribute, "atributo DN mal formado %s"},
		{oracleErrors.DNUnsupportedAttribute, "atributo DN no compatible %s"},
		{oracleErrors.DNAttributeValueMissing, "falta el valor del atributo DN %s"},
		{oracleErrors.DNMalformedEscape, "secuencia de escape DN mal formada"},
		{oracleErrors.DNMismatchAtRDN, "el DN no coincide en RDN %d"},
		{oracleErrors.DNAttributeMissingFromCertificate, "no se encontró el atributo DN configurado con OID %s en el asunto del certificado del servidor"},
		{oracleErrors.DNUnsupportedAttributeOID, "el DN configurado contiene un OID de atributo no compatible %s"},
		{oracleErrors.DNDuplicateAttributeOID, "el DN configurado contiene un OID de atributo duplicado %s"},
		{oracleErrors.DNAttributeOIDValueTypeInvalid, "el valor del atributo DN con OID %s no es una cadena"},
		{oracleErrors.DNHexValueUnsupported, "no se admiten valores de atributo DN codificados en hexadecimal"},
		{oracleErrors.DNAttributeValueInvalidUTF8, "el valor del atributo DN no es UTF-8 válido"},
		{oracleErrors.CertificateSubjectTrailingData, "el asunto del certificado del servidor contiene datos sobrantes"},
		{oracleErrors.CertificateSubjectParseFailed, "no se pudo analizar el asunto del certificado del servidor"},
		{oracleErrors.WalletCACertificatesMissing, "el wallet no contiene certificados de CA"},
		{oracleErrors.WalletCACertificatesParseFailed, "no se pudieron analizar los certificados de CA del wallet"},
		{oracleErrors.NetworkCompressionFailed, "no se pudo comprimir el paquete de red con %s"},
		{oracleErrors.NetworkDecompressionFailed, "no se pudo descomprimir el paquete de red con %s"},
		{oracleErrors.ConnectTimeout, "%s: tiempo de espera agotado después de %d para %s. (CONNECTION_ID=%s)"},
		{oracleErrors.ListenerFailed, "TNS:el listener no pudo iniciar un proceso de servidor dedicado"},
		{oracleErrors.NoDispatcherAvailable, "TNS:el listener no encontró un handler disponible con la pila de protocolos correspondiente"},
		{oracleErrors.ConnectFailed, "no se puede conectar"},
		{oracleErrors.TableOrViewNotFound, "la tabla o vista no existe"},
		{oracleErrors.InsufficientPrivilege, "privilegios insuficientes"},
		{oracleErrors.InvalidTableName, "nombre de tabla no válido"},
		{oracleErrors.MissingReadPrivilege, "falta el privilegio READ"},
		{oracleErrors.ConverterEmptyInput, "convertidor %s %s: entrada vacía"},
		{oracleErrors.ConverterRange, "convertidor %s %s: %s; esperado=%v..%v"},
		{oracleErrors.ConverterExpectedFormat, "convertidor %s %s: %s; formato esperado=%v"},
		{oracleErrors.RowDecodeError, "no se pudo decodificar %s para la columna %s (índice %d)"},
		{oracleErrors.RowsCloseFailed, "no se pudieron cerrar las filas de la instrucción: %s"},
		{oracleErrors.StatementExecutionFailed, "no se pudo preparar la instrucción %s: %s."},
		{oracleErrors.StatementExecutionFactoryFailed, "no se pudo obtener Executor de la fábrica; tipo no válido: %s para %s"},
		{oracleErrors.StatementCloseFailed, "no se pudo cerrar la instrucción: %s"},
		{oracleErrors.RunQueryError, "falló runQuery: %s."},
		{oracleErrors.RunExecError, "falló runExec: %s."},
		{oracleErrors.CallbackFactoryError, "falló la fábrica de mensajes: %s."},
		{oracleErrors.StatementParsingDanglingColon, "dos puntos sueltos al final del SQL"},
		{oracleErrors.StatementParsingPlaceholdersArgsMismatch, "se encontraron %d marcador(es) de posición, pero se proporcionaron %d argumento(s)"},
		{oracleErrors.StatementParsingInvalidArgCount, "cantidad de argumentos no válida: se proporcionaron %d, se esperaban %d marcadores de posición únicos"},
		{oracleErrors.StatementParsingNameNotFound, "no se encontró el nombre de marcador de posición %q en el SQL"},
		{oracleErrors.StatementParsingDuplicateArg, "valor duplicado para el marcador de posición %q en la aparición número %d"},
		{oracleErrors.StatementParsingInvalidOrdinal, "ordinal %d no válido; se encontraron %d marcadores de posición"},
		{oracleErrors.StatementParsingMissingValue, "falta un valor para el marcador de posición %q en la aparición número %d"},
		{oracleErrors.InvalidIdentifier, "identificador SQL no válido"},
		{oracleErrors.CancelOperationError, "no se pudo cancelar la operación"},
		{oracleErrors.InternalError, "se produjo un error interno."},
		{oracleErrors.NetworkInternalError, "error interno de red: %s"},
		{oracleErrors.MissingLocalizationService, "falta el servicio de localización en shelf"},
		{oracleErrors.UnsupportedFeature, "el controlador no admite la función: %s"},
		{oracleErrors.HTTPSProxyRequiresTCPS, "el proxy HTTPS requiere una conexión TCPS"},
		{oracleErrors.HTTPSProxyConnectFailed, "falló CONNECT del proxy HTTPS para %s"},
		{oracleErrors.InvalidSqlOutParameter, "parámetro sql.Out no válido: %s"},
		{oracleErrors.InvalidFileConfig, "no se pudo cargar la configuración"},
		{oracleErrors.NotInTransaction, "no hay ninguna transacción activa"},
		{oracleErrors.IsolationLevelNotSupported, "nivel de aislamiento no compatible"},
		{oracleErrors.AlreadyInTransaction, "nivel de aislamiento no compatible"},
		{oracleErrors.ErrorInTransaction, "se produjo un error al realizar la operación de transacción %s"},
		{oracleErrors.ConfigureTransactionError, "no se pudo configurar la transacción"},
		{oracleErrors.AuthenticatorError, "se produjo un error durante la autenticación"},
		{oracleErrors.NegotiatorError, "se produjo un error al negociar la conexión"},
		{oracleErrors.MarshalEngineError, "se produjo un error al serializar o deserializar un valor de tipo %s"},
		{oracleErrors.MarshalEngineFlushError, "se produjo un error al vaciar los datos"},
		{oracleErrors.StreamerReadError, "se produjo un error al leer un mensaje"},
		{oracleErrors.StreamerWriteError, "se produjo un error al escribir un mensaje"},
		{oracleErrors.DatabaseMountStateError, "la base de datos está en estado NOMOUNT"},
		{oracleErrors.EmptyUsernameError, "no se admiten nombres de usuario vacíos"},
		{oracleErrors.NoAuthenticatorError, "no hay un autenticador disponible para los parámetros indicados"},
		{oracleErrors.ServerTimeZoneError, "no se pudo %s la zona horaria del servidor"},
		{oracleErrors.OsonUnsupportedScalarError, "el documento OSON contiene un código de operación escalar no soportado 0x%02x"},
		{oracleErrors.JSONAccessError, "acceso JSON no válido: %s"},
		{oracleErrors.JSONUninitializedValue, "el método JSON %s se ha llamado sobre un valor sin inicializar"},
		{oracleErrors.JSONScanTypeUnsupportedError, "JSON no puede leer un valor de tipo %s"},
		{oracleErrors.JSONArrayIndexOutOfRangeError, "el índice de matriz JSON %d está fuera de rango"},
		{oracleErrors.OsonEncodingError, "no se ha podido codificar el valor como documento OSON"},
		{oracleErrors.JSONRenderingError, "no se ha podido representar el valor JSON de Oracle como texto JSON"},
		{oracleErrors.JSONObjectKeyNotFoundError, "no se ha encontrado la clave del objeto JSON"},
		{oracleErrors.InvalidConnection, "la conexión proporcionada no es una conexión de Oracle"},
		{oracleErrors.EmptyTokenError, "el ID del token para la autenticación basada en tokens está vacío"},
		{oracleErrors.TokenAuthenticationError, "se produjo un error durante la autenticación con token"},
		{oracleErrors.ValueRetrievalError, "no se pudo recuperar el valor %s"},
		{oracleErrors.InvalidSignedTokenPrivateKey, "clave privada del token firmado no válida"},
		{oracleErrors.ExpiredToken, "el token de acceso ha caducado"},
		{oracleErrors.ProviderNotFound, "no se encontró ningún proveedor del tipo solicitado"},
	}

	for _, translation := range translations {
		message.SetString(language.Spanish, string(translation.code), translation.message)
	}
}
