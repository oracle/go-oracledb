/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and data
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

package lob

import (
	"flag"
	"os"
	"strings"
	"testing"
)

// TestCategory selects the registered test category to execute.
var TestCategory string

// TestMain registers the package test-category flag before running tests.
func TestMain(m *testing.M) {
	flag.StringVar(&TestCategory, "test.category", "", "testing category, can be unitary, functional, performance, robustness")
	os.Exit(m.Run())
}

var testCases = []struct {
	name       string
	categories string
	exclusive  bool
	f          func(t *testing.T)
}{
	{"TestDirectLOB_AcceptedWriteBytes", "unitary", false, TestDirectLOB_AcceptedWriteBytes},
	{"TestDirectLOB_BlobReadCapsRefillAndConsumesPending", "unitary", false, TestDirectLOB_BlobReadCapsRefillAndConsumesPending},
	{"TestDirectLOB_IsTemporary", "unitary", false, TestDirectLOB_IsTemporary},
	{"TestDirectLOB_OpenRequiresCloseServerBeforeClose", "unitary", false, TestDirectLOB_OpenRequiresCloseServerBeforeClose},
	{"TestDirectLOB_OpenRequiresCloseServerBeforeFree", "unitary", false, TestDirectLOB_OpenRequiresCloseServerBeforeFree},
	{"TestDirectLOB_CloseServerFailurePreservesOpenState", "unitary", false, TestDirectLOB_CloseServerFailurePreservesOpenState},
	{"TestDirectLOB_OpenPersistentSerializesWithClose", "unitary", false, TestDirectLOB_OpenPersistentSerializesWithClose},
	{"TestDirectLOB_TrimClearsPendingCLOB", "unitary", false, TestDirectLOB_TrimClearsPendingCLOB},
	{"TestDirectLOB_CreateAndPromoteValidation", "unitary", false, TestDirectLOB_CreateAndPromoteValidation},
	{"TestDirectLOB_ReadWriteAndWriteTo", "unitary", false, TestDirectLOB_ReadWriteAndWriteTo},
	{"TestDirectLOB_OperationsAndLifecycle", "unitary", false, TestDirectLOB_OperationsAndLifecycle},
	{"TestLOB_ScanDelegatesOperations", "unitary", false, TestLOB_ScanDelegatesOperations},
	{"TestLOB_ScanRejectsInvalidSources", "unitary", false, TestLOB_ScanRejectsInvalidSources},
	{"TestLOB_ScanInvalidReplacementPreservesCurrentSource", "unitary", false, TestLOB_ScanInvalidReplacementPreservesCurrentSource},
	{"TestLOB_CloseIsIdempotentAndInvalidates", "unitary", false, TestLOB_CloseIsIdempotentAndInvalidates},
	{"TestLOB_ScanReplacesPreviousSource", "unitary", false, TestLOB_ScanReplacesPreviousSource},
	{"TestLOB_ScanJoinsCloseErrors", "unitary", false, TestLOB_ScanJoinsCloseErrors},
	{"TestLOB_DelegatesSourceErrors", "unitary", false, TestLOB_DelegatesSourceErrors},
	{"TestLOB_ClosePropagatesSourceError", "unitary", false, TestLOB_ClosePropagatesSourceError},
	{"TestLOB_ScanAcceptsNonPointerSource", "unitary", false, TestLOB_ScanAcceptsNonPointerSource},
	{"TestLOBScan_BytesReadsAndCloses", "unitary", false, TestLOBScan_BytesReadsAndCloses},
	{"TestLOBScan_TextReadsCharacterKindsAndCloses", "unitary", false, TestLOBScan_TextReadsCharacterKindsAndCloses},
	{"TestLOBScan_RejectsNullAndWrongKind", "unitary", false, TestLOBScan_RejectsNullAndWrongKind},
	{"TestLOBScan_RejectsTypedNilSource", "unitary", false, TestLOBScan_RejectsTypedNilSource},
	{"TestLOBScan_ScanPropagatesReadErrors", "unitary", false, TestLOBScan_ScanPropagatesReadErrors},
	{"TestLOBScan_ReadAllSizeErrors", "unitary", false, TestLOBScan_ReadAllSizeErrors},
	{"TestLOBScan_ReadAllErrors", "unitary", false, TestLOBScan_ReadAllErrors},
}

// TestCategoryExecutor runs the tests registered for the selected category.
func TestCategoryExecutor(t *testing.T) {
	var regularCases, exclusiveCases []struct {
		name       string
		categories string
		exclusive  bool
		f          func(t *testing.T)
	}

	for _, testCase := range testCases {
		for _, category := range strings.Split(testCase.categories, ",") {
			if strings.TrimSpace(category) != TestCategory {
				continue
			}
			if testCase.exclusive {
				exclusiveCases = append(exclusiveCases, testCase)
			} else {
				regularCases = append(regularCases, testCase)
			}
			break
		}
	}

	if len(regularCases) > 0 {
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			for _, testCase := range regularCases {
				t.Run(testCase.name, testCase.f)
			}
		})
	}

	for _, testCase := range exclusiveCases {
		t.Run(testCase.name, testCase.f)
	}
}
