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

// initMessagesFr initializes driver error messages in French.
func initMessagesFr() {
	translations := []struct {
		code    oracleErrors.ErrorCode
		message string
	}{
		{oracleErrors.InvalidCredential, "identifiants invalides ou utilisateur non autorisé : %s"},
		{oracleErrors.ConnectionLost, "connexion de base de données fermée par le pair"},
		{oracleErrors.AliasNotFound, "impossible de se connecter à la base de données. Alias %s introuvable."},
		{oracleErrors.NoListenerAvailable, "connexion impossible. Aucun processus d'écoute à %s"},
		{oracleErrors.ProtocolViolation, "violation de protocole"},
		{oracleErrors.ProtocolViolationLimitExceeded, "violation de protocole : limite %s dépassée : limite [%d], valeur [%d]"},
		{oracleErrors.LobClosed, "LOB déjà fermé dans la même transaction."},
		{oracleErrors.LobOpen, "LOB déjà ouvert dans la même transaction."},
		{oracleErrors.ChannelReadFailed, "fin de flux détectée. Le pair distant a fermé la connexion."},
		{oracleErrors.ChannelWriteFailed, "échec de l'écriture sur le canal."},
		{oracleErrors.UnknownHost, "hôte spécifié inconnu."},
		{oracleErrors.LogonDenied, "identifiants invalides ou autorisation refusée"},
		{oracleErrors.InvalidServiceName, "impossible de se connecter à la base de données. Le service %s n'est pas enregistré auprès de l'écouteur sur l'hôte %s, port %d. (CONNECTION_ID=%s)"},
		{oracleErrors.NoAvailableHandler, "impossible de se connecter à la base de données. Aucun gestionnaire %s disponible pour le service demandé %s sur l'hôte %s, port %d. (CONNECTION_ID=%s)"},
		{oracleErrors.UnknownInstance, "impossible de se connecter à la base de données. L'instance %s du service %s n'est pas connue de l'écouteur sur l'hôte %s, port %d. (CONNECTION_ID=%s)"},
		{oracleErrors.UnresolvedAlias, "impossible de résoudre l'alias de connexion en raison d'erreurs dans la syntaxe Easy Connect %s"},
		{oracleErrors.InvalidSID, "impossible de se connecter à la base de données. Le SID %s n'est pas enregistré auprès de l'écouteur sur l'hôte %s, port %d. (CONNECTION_ID=%s)"},
		{oracleErrors.ListenerRequestTimeout, "l'écouteur n'a pas reçu la requête du client dans le délai autorisé"},
		{oracleErrors.ConnectionClosed, "connexion fermée"},
		{oracleErrors.CtxTimeout, "délai d'attente du contexte dépassé pendant %s pour %s (CONNECTION_ID=%s)"},
		{oracleErrors.ConnCloseTimeout, "un délai d'attente s'est produit pendant la fermeture de la session réseau"},
		{oracleErrors.UnknownSPFFunction, "code de fonction inconnu reçu du serveur vers le client : %d"},
		{oracleErrors.SPFNotFunction, "le message n'implémente pas l'interface Function"},
		{oracleErrors.InvalidSPFFunction, "le message n'est pas du type attendu : [%s]"},
		{oracleErrors.InvalidConnectionParameter, "valeur [%s] non valide pour le paramètre de connexion [%s] ; valeurs acceptées : [%s]"},
		{oracleErrors.ConflictingConnectionParameterSource, "le paramètre [%s] ne peut pas être spécifié dans plusieurs sources"},
		{oracleErrors.NamingDSNInvalid, "impossible d'analyser %s dans le DSN"},
		{oracleErrors.NamingParseFailed, "chaîne de connexion non valide"},
		{oracleErrors.NamingParsePosition, "nom attendu après '(' à la position %d"},
		{oracleErrors.NamingParseValue, "élément non valide %q dans la chaîne de connexion"},
		{oracleErrors.NamingParseValues, "chaîne de connexion non valide : racine obtenue %q, racine attendue %q"},
		{oracleErrors.NamingParsePath, "chemin non valide %q dans la chaîne de connexion"},
		{oracleErrors.NamingParsePathSegment, "segment de chemin non valide %q dans %q à la position %d"},
		{oracleErrors.NamingParseBounds, "limites de la chaîne de connexion non valides : valeur obtenue %d, valeur attendue %d"},
		{oracleErrors.NamingEzConnectError, "valeur %q non valide pour %s dans la chaîne de connexion"},
		{oracleErrors.NamingContextError, "valeur %q non valide pour %s dans le descripteur de connexion"},
		{oracleErrors.FailMarshal, "échec de la sérialisation du message : %s"},
		{oracleErrors.FailUnmarshal, "échec de la désérialisation du message : %s"},
		{oracleErrors.InvalidLOBBuffer, "configuration de tampon LOB non valide pendant %s (%s) : %s"},
		{oracleErrors.LobExecError, "échec de l'opération LOB pendant %s."},
		{oracleErrors.UnsupportedCharacterSet, "jeu de caractères non pris en charge : %d"},
		{oracleErrors.InvalidNetworkValue, "valeur non valide pour %s : %v"},
		{oracleErrors.InvalidNetworkExpectedValue, "valeur non valide pour %s : obtenue %v, attendue %v"},
		{oracleErrors.InvalidNetworkContextValue, "valeur non valide pour %s (%s) : %v"},
		{oracleErrors.InvalidNetworkContextExpectedValue, "valeur non valide pour %s (%s) : obtenue %v, attendue %v"},
		{oracleErrors.InvalidNetworkLength, "longueur non valide pour %s : %d"},
		{oracleErrors.InvalidNetworkExpectedLength, "longueur non valide pour %s : obtenue %d, attendue %d"},
		{oracleErrors.InvalidNetworkContextExpectedLength, "longueur non valide pour %s (%s) : obtenue %d, attendue %d"},
		{oracleErrors.ErrConnectionInband, "erreur du serveur reçue en bande"},
		{oracleErrors.NetworkDataReadFailed, "échec de la lecture des données réseau"},
		{oracleErrors.ConnectionRefusedDetail, "connexion refusée : ORA-%s, motif utilisateur %d, motif système %d"},
		{oracleErrors.NetworkServerErrorCode, "%s-%s"},
		{oracleErrors.BreakPacketReceived, "paquet d'interruption reçu"},
		{oracleErrors.RefuseDataParseFailed, "échec de l'analyse des données de refus"},
		{oracleErrors.RedirectAddressMissing, "les données de redirection ne contiennent aucune adresse"},
		{oracleErrors.TLSRenegotiationUnsupported, "la renégociation TLS n'est pas prise en charge par cette connexion"},
		{oracleErrors.UnexpectedConnectResponse, "paquet inattendu pendant la connexion"},
		{oracleErrors.NamingInputMissing, "%s manquant"},
		{oracleErrors.NamingTokensMissing, "la chaîne de connexion ne contient aucun élément valide"},
		{oracleErrors.NamingUnexpectedClosingParenthesis, "parenthèse fermante ')' inattendue à la position %d"},
		{oracleErrors.NetworkRetryLimitExceeded, "trop de %s : maximum de %d dépassé"},
		{oracleErrors.InvalidNetworkProperty, "valeur non valide pour la propriété %s"},
		{oracleErrors.PKCS8ParseFailed, "échec de l'analyse de la clé privée chiffrée PKCS#8"},
		{oracleErrors.PKCS8DecryptFailed, "échec du déchiffrement de la clé privée chiffrée PKCS#8"},
		{oracleErrors.PKCS7PaddingInvalid, "bourrage PKCS#7 non valide"},
		{oracleErrors.TLSCertificateVerificationFailed, "échec de la vérification du certificat du serveur"},
		{oracleErrors.TLSCertificateDNMatchFailed, "le DN du certificat du serveur ne correspond pas"},
		{oracleErrors.PEMBlockDecryptFailed, "échec du déchiffrement du bloc PEM"},
		{oracleErrors.WalletKeyPairLoadFailed, "échec du chargement de la paire de clés du wallet"},
		{oracleErrors.ConfiguredDNInvalid, "DN configuré non valide"},
		{oracleErrors.CertificateSubjectInvalid, "sujet du certificat du serveur non valide"},
		{oracleErrors.PKCS8ParametersParseFailed, "échec de l'analyse des paramètres %s"},
		{oracleErrors.PKCS8InitializationVectorParseFailed, "échec de l'analyse du vecteur d'initialisation PKCS#8"},
		{oracleErrors.DNMalformedAttribute, "attribut DN mal formé %s"},
		{oracleErrors.DNUnsupportedAttribute, "attribut DN non pris en charge %s"},
		{oracleErrors.DNAttributeValueMissing, "valeur manquante pour l'attribut DN %s"},
		{oracleErrors.DNMalformedEscape, "séquence d'échappement DN mal formée"},
		{oracleErrors.DNMismatchAtRDN, "différence de DN au RDN %d"},
		{oracleErrors.DNAttributeMissingFromCertificate, "l'attribut DN configuré avec l'OID %s est introuvable dans le sujet du certificat du serveur"},
		{oracleErrors.DNUnsupportedAttributeOID, "le DN configuré contient un OID d'attribut non pris en charge %s"},
		{oracleErrors.DNDuplicateAttributeOID, "le DN configuré contient un OID d'attribut en double %s"},
		{oracleErrors.DNAttributeOIDValueTypeInvalid, "la valeur de l'attribut DN avec l'OID %s n'est pas une chaîne"},
		{oracleErrors.DNHexValueUnsupported, "les valeurs d'attribut DN encodées en hexadécimal ne sont pas prises en charge"},
		{oracleErrors.DNAttributeValueInvalidUTF8, "la valeur de l'attribut DN n'est pas un UTF-8 valide"},
		{oracleErrors.CertificateSubjectTrailingData, "le sujet du certificat du serveur contient des données supplémentaires"},
		{oracleErrors.CertificateSubjectParseFailed, "échec de l'analyse du sujet du certificat du serveur"},
		{oracleErrors.WalletCACertificatesMissing, "le wallet ne contient aucun certificat d'autorité de certification"},
		{oracleErrors.WalletCACertificatesParseFailed, "échec de l'analyse des certificats d'autorité de certification du wallet"},
		{oracleErrors.NetworkCompressionFailed, "échec de la compression du paquet réseau avec %s"},
		{oracleErrors.NetworkDecompressionFailed, "échec de la décompression du paquet réseau avec %s"},
		{oracleErrors.ConnectTimeout, "délai d'attente %s de %d pour %s.(CONNECTION_ID=%s)"},
		{oracleErrors.ListenerFailed, "TNS:échec du démarrage d'un processus serveur dédié par l'écouteur"},
		{oracleErrors.NoDispatcherAvailable, "TNS:l'écouteur n'a pas trouvé de gestionnaire disponible avec la pile de protocoles correspondante"},
		{oracleErrors.ConnectFailed, "connexion impossible"},
		{oracleErrors.TableOrViewNotFound, "la table ou la vue n'existe pas"},
		{oracleErrors.InsufficientPrivilege, "privilèges insuffisants"},
		{oracleErrors.InvalidTableName, "nom de table non valide"},
		{oracleErrors.MissingReadPrivilege, "privilège READ manquant"},
		{oracleErrors.ConverterEmptyInput, "convertisseur %s %s : entrée vide"},
		{oracleErrors.ConverterRange, "convertisseur %s %s : %s ; attendu=%v..%v"},
		{oracleErrors.ConverterExpectedFormat, "convertisseur %s %s : %s ; format attendu=%v"},
		{oracleErrors.RowDecodeError, "échec du décodage de %s pour la colonne %s (index %d)"},
		{oracleErrors.RowsCloseFailed, "échec de la fermeture des lignes de l'instruction : %s"},
		{oracleErrors.StatementExecutionFailed, "échec de la préparation de l'instruction %s : %s."},
		{oracleErrors.StatementExecutionFactoryFailed, "échec de l'obtention de l'Executor depuis la fabrique, type non valide : %s pour %s"},
		{oracleErrors.StatementCloseFailed, "échec de la fermeture de l'instruction : %s"},
		{oracleErrors.RunQueryError, "échec de runQuery : %s."},
		{oracleErrors.RunExecError, "échec de runExec : %s."},
		{oracleErrors.CallbackFactoryError, "échec de la fabrique de messages : %s."},
		{oracleErrors.StatementParsingDanglingColon, "deux-points sans identifiant à la fin du SQL"},
		{oracleErrors.StatementParsingPlaceholdersArgsMismatch, "%d espace(s) réservé(s) trouvé(s), mais %d argument(s) fourni(s)"},
		{oracleErrors.StatementParsingInvalidArgCount, "nombre d'arguments non valide : %d fourni(s), %d espace(s) réservé(s) unique(s) attendu(s)"},
		{oracleErrors.StatementParsingNameNotFound, "nom d'espace réservé %q introuvable dans le SQL"},
		{oracleErrors.StatementParsingDuplicateArg, "valeur en double pour l'espace réservé %q à l'occurrence numéro %d"},
		{oracleErrors.StatementParsingInvalidOrdinal, "numéro d'ordre %d non valide ; nombre d'espaces réservés trouvés : %d"},
		{oracleErrors.StatementParsingMissingValue, "valeur manquante pour l'espace réservé %q à l'occurrence numéro %d"},
		{oracleErrors.InvalidIdentifier, "identifiant SQL non valide"},
		{oracleErrors.CancelOperationError, "échec de l'annulation de l'opération"},
		{oracleErrors.InternalError, "une erreur interne s'est produite."},
		{oracleErrors.NetworkInternalError, "erreur réseau interne : %s"},
		{oracleErrors.MissingLocalizationService, "service de localisation manquant sur la shelf"},
		{oracleErrors.UnsupportedFeature, "fonctionnalité non prise en charge par le pilote : %s"},
		{oracleErrors.HTTPSProxyRequiresTCPS, "le proxy HTTPS exige une connexion TCPS"},
		{oracleErrors.HTTPSProxyConnectFailed, "échec de la requête CONNECT du proxy HTTPS pour %s"},
		{oracleErrors.InvalidSqlOutParameter, "paramètre sql.Out non valide : %s"},
		{oracleErrors.InvalidFileConfig, "échec du chargement de la configuration"},
		{oracleErrors.NotInTransaction, "aucune transaction en cours"},
		{oracleErrors.IsolationLevelNotSupported, "niveau d'isolation non pris en charge"},
		{oracleErrors.AlreadyInTransaction, "niveau d'isolation non pris en charge"},
		{oracleErrors.ErrorInTransaction, "une erreur s'est produite pendant l'opération de transaction %s"},
		{oracleErrors.ConfigureTransactionError, "échec de la configuration de la transaction"},
		{oracleErrors.AuthenticatorError, "une erreur s'est produite pendant l'authentification"},
		{oracleErrors.NegotiatorError, "une erreur s'est produite pendant la négociation de la connexion"},
		{oracleErrors.MarshalEngineError, "une erreur s'est produite pendant la sérialisation ou la désérialisation d'une valeur de type %s"},
		{oracleErrors.MarshalEngineFlushError, "une erreur s'est produite pendant le vidage des données"},
		{oracleErrors.StreamerReadError, "une erreur s'est produite pendant la lecture d'un message"},
		{oracleErrors.StreamerWriteError, "une erreur s'est produite pendant l'écriture d'un message"},
		{oracleErrors.DatabaseMountStateError, "la base de données est à l'état NOMOUNT"},
		{oracleErrors.EmptyUsernameError, "les noms d'utilisateur vides ne sont pas pris en charge"},
		{oracleErrors.NoAuthenticatorError, "aucun authentificateur disponible pour les paramètres fournis"},
		{oracleErrors.ServerTimeZoneError, "échec de l'opération %s sur le fuseau horaire du serveur"},
		{oracleErrors.OsonUnsupportedScalarError, "le document OSON contient un opcode scalaire non pris en charge 0x%02x"},
		{oracleErrors.JSONAccessError, "accès JSON non valide : %s"},
		{oracleErrors.JSONUninitializedValue, "la méthode JSON %s a été appelée sur une valeur non initialisée"},
		{oracleErrors.JSONScanTypeUnsupportedError, "JSON ne peut pas lire une valeur de type %s"},
		{oracleErrors.JSONArrayIndexOutOfRangeError, "l'indice de tableau JSON %d est hors limites"},
		{oracleErrors.OsonEncodingError, "échec de l'encodage de la valeur en document OSON"},
		{oracleErrors.JSONRenderingError, "échec du rendu de la valeur JSON Oracle en texte JSON"},
		{oracleErrors.JSONObjectKeyNotFoundError, "clé d'objet JSON introuvable"},
		{oracleErrors.InvalidConnection, "la connexion fournie n'est pas une connexion Oracle"},
		{oracleErrors.EmptyTokenError, "l'identifiant du jeton pour l'authentification par jeton est vide"},
		{oracleErrors.TokenAuthenticationError, "une erreur s'est produite pendant l'authentification par jeton"},
		{oracleErrors.ValueRetrievalError, "échec de la récupération de la valeur %s"},
		{oracleErrors.InvalidSignedTokenPrivateKey, "clé privée de jeton signé non valide"},
		{oracleErrors.ExpiredToken, "le jeton d'accès a expiré"},
		{oracleErrors.ProviderNotFound, "aucun fournisseur du type demandé n'a été trouvé"},
		{oracleErrors.ImplicitResultMessageCreationFailed, "échec de création du message de résultat implicite"},
		{oracleErrors.RefCursorConnectionMismatch, "le REF CURSOR doit être récupéré à l’aide de la connexion sur laquelle il a été ouvert"},
		{oracleErrors.InvalidRefCursorOutParameter, "les liaisons OUT REF CURSOR nécessitent *datatype.Cursor"},
		{oracleErrors.InvalidRefCursorArgument, "argument REF CURSOR non valide"},
		{oracleErrors.MissingReadPrivilege, "privilège READ manquant"},
	}

	for _, translation := range translations {
		message.SetString(language.French, string(translation.code), translation.message)
	}
}
