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
package tests

import (
	"strings"
	"testing"
)

type CategorizedTestCase struct {
	Name       string
	Categories string
	Exclusive  bool
	Fn         func(t *testing.T)
}

// isTestCategoryEnabled checks that the comma separated list of categories in testCategories
// matches what's enabled in categories.
func isTestCategoryEnabled(testCategories string, categories TestCategoryList) bool {
	stc := strings.Split(testCategories, ",")
	for _, ec := range stc {
		for _, c := range categories {
			if strings.EqualFold(
				strings.TrimSpace(ec),
				strings.TrimSpace(c),
			) {
				return true
			}
		}
	}
	return false
}

func RunCategoryExecutor(t *testing.T, categories TestCategoryList, cases []CategorizedTestCase) {
	var regularCases []CategorizedTestCase
	var exclusiveCases []CategorizedTestCase

	for _, c := range cases {
		if isTestCategoryEnabled(c.Categories, categories) {
			if c.Exclusive {
				exclusiveCases = append(exclusiveCases, c)
			} else {
				regularCases = append(regularCases, c)
			}
		} else {
			t.Logf("No enabled category for test case %s", c.Name)
		}
	}

	if len(regularCases) > 0 {
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			for _, c := range regularCases {
				t.Run(c.Name, c.Fn)
			}
		})
	}

	for _, c := range exclusiveCases {
		t.Run(c.Name, c.Fn)
	}

}
