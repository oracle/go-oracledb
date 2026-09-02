package driver

import (
	"testing"

	oracleTest "github.com/oracle/go-oracledb/v26/internal/tests"
)

var testCases = []oracleTest.CategorizedTestCase{
	{Name: "TestGetConnectionInstantiator", Categories: "unitary", Exclusive: false, Fn: TestGetConnectionInstantiator},
}

func TestCategoryExecutor(t *testing.T) {
	oracleTest.RunCategoryExecutor(t, oracleTest.TestCategory, testCases)
}
