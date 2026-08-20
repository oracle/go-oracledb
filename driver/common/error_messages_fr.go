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
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// initMessagesEn Initialises error messages for French language.
func initMessagesFr() {
	message.SetString(language.French, string(ConnectionLost), "connexion de base de donn\u00E9es ferm\u00E9e par l'homologue")
	message.SetString(language.French, string(AliasNotFound), "impossible d''\u00E9tablir la connexion \u00E0 la base de donn\u00E9es. Alias {0} introuvable dans {1}.")
	message.SetString(language.French, string(NoListenerAvailable), "connexion impossible. Aucun processus d''\u00E9coute dans {0}.")
	message.SetString(language.French, string(ProtocolViolation), "violation de protocole")
	message.SetString(language.French, string(UnknownHost), "h\u00F4te inconnu sp\u00E9cifi\u00E9.")
	message.SetString(language.French, string(LogonDenied), "FR invalid credential or not authorized")
	message.SetString(language.French, string(TableOrViewNotFound), "table ou vue n'existe pas")
	message.SetString(language.French, string(InsufficientPrivilege), "privilÃ¨ges insuffisants")
	message.SetString(language.French, string(InvalidTableName), "nom de table invalide")
	message.SetString(language.French, string(InvalidIdentifier), "identifiant SQL invalide")
	message.SetString(language.French, string(MissingReadPrivilege), "privilÃ¨ge READ manquant")
	message.SetString(language.French, string(MissingLocalizationService), "service de localisation manquant sur la shelf")
}
