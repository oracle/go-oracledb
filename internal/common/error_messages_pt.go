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

// initMessagesPt initializes driver error messages in Portuguese.
func initMessagesPt() {
	translations := []struct {
		code    oracleErrors.ErrorCode
		message string
	}{
		{oracleErrors.InvalidCredential, "credencial inválida ou usuário não autorizado: %s"},
		{oracleErrors.ConnectionLost, "conexão com o banco de dados encerrada pelo peer"},
		{oracleErrors.AliasNotFound, "não foi possível conectar ao banco de dados. Alias %s não encontrado."},
		{oracleErrors.NoListenerAvailable, "não foi possível conectar. Nenhum listener em %s"},
		{oracleErrors.ProtocolViolation, "violação de protocolo"},
		{oracleErrors.ProtocolViolationLimitExceeded, "violação de protocolo: limite de %s excedido: limite [%d], valor [%d]"},
		{oracleErrors.LobClosed, "LOB já fechado na mesma transação."},
		{oracleErrors.LobOpen, "LOB já aberto na mesma transação."},
		{oracleErrors.ChannelReadFailed, "fim do fluxo detectado. O peer remoto encerrou a conexão."},
		{oracleErrors.ChannelWriteFailed, "falha ao gravar no canal."},
		{oracleErrors.UnknownHost, "host especificado desconhecido."},
		{oracleErrors.LogonDenied, "credencial inválida ou usuário não autorizado"},
		{oracleErrors.InvalidServiceName, "não foi possível conectar ao banco de dados. O serviço %s não está registrado no listener do host %s, porta %d. (CONNECTION_ID=%s)"},
		{oracleErrors.NoAvailableHandler, "não foi possível conectar ao banco de dados. Nenhum handler %s disponível para o serviço solicitado %s no host %s, porta %d. (CONNECTION_ID=%s)"},
		{oracleErrors.UnknownInstance, "não foi possível conectar ao banco de dados. A instância %s do serviço %s não é conhecida pelo listener no host %s, porta %d. (CONNECTION_ID=%s)"},
		{oracleErrors.UnresolvedAlias, "não foi possível resolver o alias de conexão devido a erros na sintaxe Easy Connect %s"},
		{oracleErrors.InvalidSID, "não foi possível conectar ao banco de dados. O SID %s não está registrado no listener do host %s, porta %d. (CONNECTION_ID=%s)"},
		{oracleErrors.ListenerRequestTimeout, "o listener não recebeu a solicitação do cliente dentro do tempo permitido"},
		{oracleErrors.ConnectionClosed, "conexão encerrada"},
		{oracleErrors.CtxTimeout, "o contexto expirou durante %s para %s (CONNECTION_ID=%s)"},
		{oracleErrors.ConnCloseTimeout, "ocorreu um tempo limite ao encerrar a sessão de rede"},
		{oracleErrors.UnknownSPFFunction, "código de função servidor-cliente desconhecido recebido: %d"},
		{oracleErrors.SPFNotFunction, "a mensagem não implementa a interface Function"},
		{oracleErrors.InvalidSPFFunction, "a mensagem não é do tipo esperado: [%s]"},
		{oracleErrors.InvalidConnectionParameter, "valor inválido [%s] para o parâmetro de conexão [%s]; valores aceitos: [%s]"},
		{oracleErrors.ConflictingConnectionParameterSource, "o parâmetro [%s] não pode ser especificado em mais de uma origem"},
		{oracleErrors.NamingDSNInvalid, "não foi possível analisar %s do DSN"},
		{oracleErrors.NamingParseFailed, "string de conexão inválida"},
		{oracleErrors.NamingParsePosition, "nome esperado após '(' na posição %d"},
		{oracleErrors.NamingParseValue, "token inválido %q na string de conexão"},
		{oracleErrors.NamingParseValues, "string de conexão inválida: raiz recebida %q, esperada %q"},
		{oracleErrors.NamingParsePath, "caminho inválido %q na string de conexão"},
		{oracleErrors.NamingParsePathSegment, "segmento de caminho inválido %q em %q na posição %d"},
		{oracleErrors.NamingParseBounds, "limites inválidos na string de conexão: recebido %d, esperado %d"},
		{oracleErrors.NamingEzConnectError, "valor inválido %q para %s na string de conexão"},
		{oracleErrors.NamingContextError, "valor inválido %q para %s no descritor de conexão"},
		{oracleErrors.FailMarshal, "falha ao serializar a mensagem: %s"},
		{oracleErrors.FailUnmarshal, "falha ao desserializar a mensagem: %s"},
		{oracleErrors.InvalidLOBBuffer, "configuração inválida do buffer LOB durante %s (%s): %s"},
		{oracleErrors.LobExecError, "falha na operação LOB durante %s."},
		{oracleErrors.UnsupportedCharacterSet, "conjunto de caracteres não compatível: %d"},
		{oracleErrors.InvalidNetworkValue, "valor inválido para %s: %v"},
		{oracleErrors.InvalidNetworkExpectedValue, "valor inválido para %s: recebido %v, esperado %v"},
		{oracleErrors.InvalidNetworkContextValue, "valor inválido para %s (%s): %v"},
		{oracleErrors.InvalidNetworkContextExpectedValue, "valor inválido para %s (%s): recebido %v, esperado %v"},
		{oracleErrors.InvalidNetworkLength, "comprimento inválido para %s: %d"},
		{oracleErrors.InvalidNetworkExpectedLength, "comprimento inválido para %s: recebido %d, esperado %d"},
		{oracleErrors.InvalidNetworkContextExpectedLength, "comprimento inválido para %s (%s): recebido %d, esperado %d"},
		{oracleErrors.ErrConnectionInband, "erro recebido do servidor em banda"},
		{oracleErrors.NetworkDataReadFailed, "falha ao ler os dados da rede"},
		{oracleErrors.ConnectionRefusedDetail, "conexão recusada: ORA-%s, motivo do usuário %d, motivo do sistema %d"},
		{oracleErrors.NetworkServerErrorCode, "%s-%s"},
		{oracleErrors.BreakPacketReceived, "pacote de interrupção recebido"},
		{oracleErrors.RefuseDataParseFailed, "falha ao analisar os dados de recusa"},
		{oracleErrors.RedirectAddressMissing, "os dados de redirecionamento não contêm nenhum endereço"},
		{oracleErrors.TLSRenegotiationUnsupported, "esta conexão não oferece suporte à renegociação TLS"},
		{oracleErrors.UnexpectedConnectResponse, "pacote inesperado durante a conexão"},
		{oracleErrors.NamingInputMissing, "ausência de %s"},
		{oracleErrors.NamingTokensMissing, "a string de conexão não contém tokens válidos"},
		{oracleErrors.NamingUnexpectedClosingParenthesis, "parêntese de fechamento ')' inesperado na posição %d"},
		{oracleErrors.NetworkRetryLimitExceeded, "quantidade excessiva de %s: limite de %d excedido"},
		{oracleErrors.InvalidNetworkProperty, "valor inválido para a propriedade %s"},
		{oracleErrors.PKCS8ParseFailed, "falha ao analisar a chave privada criptografada PKCS#8"},
		{oracleErrors.PKCS8DecryptFailed, "falha ao descriptografar a chave privada criptografada PKCS#8"},
		{oracleErrors.PKCS7PaddingInvalid, "preenchimento PKCS#7 inválido"},
		{oracleErrors.TLSCertificateVerificationFailed, "falha ao verificar o certificado do servidor"},
		{oracleErrors.TLSCertificateDNMatchFailed, "o DN do certificado do servidor não corresponde"},
		{oracleErrors.PEMBlockDecryptFailed, "falha ao descriptografar o bloco PEM"},
		{oracleErrors.WalletKeyPairLoadFailed, "falha ao carregar o par de chaves do wallet"},
		{oracleErrors.ConfiguredDNInvalid, "DN configurado inválido"},
		{oracleErrors.CertificateSubjectInvalid, "assunto do certificado do servidor inválido"},
		{oracleErrors.PKCS8ParametersParseFailed, "falha ao analisar os parâmetros %s"},
		{oracleErrors.PKCS8InitializationVectorParseFailed, "falha ao analisar o vetor de inicialização PKCS#8"},
		{oracleErrors.DNMalformedAttribute, "atributo DN malformado %s"},
		{oracleErrors.DNUnsupportedAttribute, "atributo DN não compatível %s"},
		{oracleErrors.DNAttributeValueMissing, "valor ausente para o atributo DN %s"},
		{oracleErrors.DNMalformedEscape, "sequência de escape DN malformada"},
		{oracleErrors.DNMismatchAtRDN, "o DN não corresponde ao RDN %d"},
		{oracleErrors.DNAttributeMissingFromCertificate, "o atributo DN configurado com OID %s não foi encontrado no assunto do certificado do servidor"},
		{oracleErrors.DNUnsupportedAttributeOID, "o DN configurado contém um OID de atributo não compatível %s"},
		{oracleErrors.DNDuplicateAttributeOID, "o DN configurado contém um OID de atributo duplicado %s"},
		{oracleErrors.DNAttributeOIDValueTypeInvalid, "o valor do atributo DN com OID %s não é uma string"},
		{oracleErrors.DNHexValueUnsupported, "valores de atributo DN codificados em hexadecimal não são compatíveis"},
		{oracleErrors.DNAttributeValueInvalidUTF8, "o valor do atributo DN não é UTF-8 válido"},
		{oracleErrors.CertificateSubjectTrailingData, "o assunto do certificado do servidor contém dados excedentes"},
		{oracleErrors.CertificateSubjectParseFailed, "falha ao analisar o assunto do certificado do servidor"},
		{oracleErrors.WalletCACertificatesMissing, "o wallet não contém certificados de CA"},
		{oracleErrors.WalletCACertificatesParseFailed, "falha ao analisar os certificados de CA do wallet"},
		{oracleErrors.NetworkCompressionFailed, "falha ao compactar o pacote de rede com %s"},
		{oracleErrors.NetworkDecompressionFailed, "falha ao descompactar o pacote de rede com %s"},
		{oracleErrors.ConnectTimeout, "%s: tempo limite de %d excedido para %s. (CONNECTION_ID=%s)"},
		{oracleErrors.ListenerFailed, "TNS: o listener falhou ao iniciar um processo de servidor dedicado"},
		{oracleErrors.NoDispatcherAvailable, "TNS: o listener não encontrou um handler disponível com a pilha de protocolos correspondente"},
		{oracleErrors.ConnectFailed, "não foi possível conectar"},
		{oracleErrors.TableOrViewNotFound, "a tabela ou visualização não existe"},
		{oracleErrors.InsufficientPrivilege, "privilégios insuficientes"},
		{oracleErrors.InvalidTableName, "nome de tabela inválido"},
		{oracleErrors.MissingReadPrivilege, "privilégio READ ausente"},
		{oracleErrors.ConverterEmptyInput, "conversor %s %s: entrada vazia"},
		{oracleErrors.ConverterRange, "conversor %s %s: %s; esperado=%v..%v"},
		{oracleErrors.ConverterExpectedFormat, "conversor %s %s: %s; formato esperado=%v"},
		{oracleErrors.RowDecodeError, "falha ao decodificar %s para a coluna %s (índice %d)"},
		{oracleErrors.RowsCloseFailed, "falha ao fechar as linhas da instrução: %s"},
		{oracleErrors.StatementExecutionFailed, "falha ao preparar a instrução %s: %s."},
		{oracleErrors.StatementExecutionFactoryFailed, "falha ao obter Executor da fábrica; tipo inválido: %s para %s"},
		{oracleErrors.StatementCloseFailed, "falha ao fechar a instrução: %s"},
		{oracleErrors.RunQueryError, "falha em runQuery: %s."},
		{oracleErrors.RunExecError, "falha em runExec: %s."},
		{oracleErrors.CallbackFactoryError, "falha na fábrica de mensagens: %s."},
		{oracleErrors.StatementParsingDanglingColon, "dois-pontos sem identificador ao final do SQL"},
		{oracleErrors.StatementParsingPlaceholdersArgsMismatch, "foram encontrados %d espaço(s) reservado(s), mas foram fornecido(s) %d argumento(s)"},
		{oracleErrors.StatementParsingInvalidArgCount, "quantidade inválida de argumentos: %d fornecido(s), esperado(s) %d espaço(s) reservado(s) único(s)"},
		{oracleErrors.StatementParsingNameNotFound, "nome do espaço reservado %q não encontrado no SQL"},
		{oracleErrors.StatementParsingDuplicateArg, "valor duplicado para o espaço reservado %q na ocorrência número %d"},
		{oracleErrors.StatementParsingInvalidOrdinal, "ordinal %d inválido; encontrados %d espaços reservados"},
		{oracleErrors.StatementParsingMissingValue, "valor ausente para o espaço reservado %q na ocorrência número %d"},
		{oracleErrors.InvalidIdentifier, "identificador SQL inválido"},
		{oracleErrors.CancelOperationError, "falha ao cancelar a operação"},
		{oracleErrors.InternalError, "ocorreu um erro interno."},
		{oracleErrors.NetworkInternalError, "erro interno de rede: %s"},
		{oracleErrors.MissingLocalizationService, "serviço de localização ausente na shelf"},
		{oracleErrors.UnsupportedFeature, "recurso não compatível com o driver: %s"},
		{oracleErrors.HTTPSProxyRequiresTCPS, "o proxy HTTPS requer uma conexão TCPS"},
		{oracleErrors.HTTPSProxyConnectFailed, "falha na solicitação CONNECT do proxy HTTPS para %s"},
		{oracleErrors.InvalidSqlOutParameter, "parâmetro sql.Out inválido: %s"},
		{oracleErrors.InvalidFileConfig, "falha ao carregar a configuração"},
		{oracleErrors.NotInTransaction, "nenhuma transação ativa"},
		{oracleErrors.IsolationLevelNotSupported, "nível de isolamento não compatível"},
		{oracleErrors.AlreadyInTransaction, "a transação já foi iniciada"},
		{oracleErrors.ErrorInTransaction, "ocorreu um erro durante a operação de transação %s"},
		{oracleErrors.ConfigureTransactionError, "falha ao configurar a transação"},
		{oracleErrors.AuthenticatorError, "ocorreu um erro durante a autenticação"},
		{oracleErrors.NegotiatorError, "ocorreu um erro durante a negociação da conexão"},
		{oracleErrors.MarshalEngineError, "ocorreu um erro ao serializar ou desserializar um valor do tipo %s"},
		{oracleErrors.MarshalEngineFlushError, "ocorreu um erro ao descarregar os dados"},
		{oracleErrors.StreamerReadError, "ocorreu um erro ao ler uma mensagem"},
		{oracleErrors.StreamerWriteError, "ocorreu um erro ao gravar uma mensagem"},
		{oracleErrors.DatabaseMountStateError, "o banco de dados está no estado NOMOUNT"},
		{oracleErrors.EmptyUsernameError, "nomes de usuário vazios não são compatíveis"},
		{oracleErrors.NoAuthenticatorError, "nenhum autenticador disponível para os parâmetros fornecidos"},
		{oracleErrors.ServerTimeZoneError, "falha ao %s o fuso horário do servidor"},
		{oracleErrors.OsonUnsupportedScalarError, "o documento OSON contém um código de operação escalar não suportado 0x%02x"},
		{oracleErrors.JSONAccessError, "acesso JSON inválido: %s"},
		{oracleErrors.JSONUninitializedValue, "o método JSON %s foi chamado em um valor não inicializado"},
		{oracleErrors.JSONScanTypeUnsupportedError, "JSON não pode ler um valor do tipo %s"},
		{oracleErrors.JSONArrayIndexOutOfRangeError, "o índice de array JSON %d está fora dos limites"},
		{oracleErrors.OsonEncodingError, "falha ao codificar o valor como documento OSON"},
		{oracleErrors.JSONRenderingError, "falha ao representar o valor JSON do Oracle como texto JSON"},
		{oracleErrors.JSONObjectKeyNotFoundError, "chave do objeto JSON não encontrada"},
		{oracleErrors.InvalidConnection, "a conexão fornecida não é uma conexão Oracle"},
		{oracleErrors.EmptyTokenError, "o ID do token para autenticação baseada em token está vazio"},
		{oracleErrors.TokenAuthenticationError, "ocorreu um erro durante a autenticação por token"},
		{oracleErrors.ValueRetrievalError, "falha ao obter o valor %s"},
		{oracleErrors.InvalidSignedTokenPrivateKey, "chave privada de token assinado inválida"},
		{oracleErrors.ExpiredToken, "o token de acesso expirou"},
		{oracleErrors.ProviderNotFound, "nenhum provedor do tipo solicitado foi encontrado"},
	}

	for _, translation := range translations {
		message.SetString(language.Portuguese, string(translation.code), translation.message)
	}
}
