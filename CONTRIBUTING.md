# Contributing to this repository

We welcome your contributions! There are multiple ways to contribute.

## Opening issues

For bugs or enhancement requests, please file a GitHub issue unless it's
security related. When filing a bug remember that the better written the bug is,
the more likely it is to be fixed. If you think you've found a security
vulnerability, do not raise a GitHub issue and follow the instructions in our
[security policy](./SECURITY.md).

## Contributing code

We welcome your code contributions. Before submitting code via a pull request,
you will need to have signed the [Oracle Contributor Agreement][OCA] (OCA) and
your commits need to include the following line using the name and e-mail
address you used to sign the OCA:

```text
Signed-off-by: Your Name <you@example.org>
```

This can be automatically added to pull requests by committing with `--sign-off`
or `-s`, e.g.

```text
git commit --signoff
```

Only pull requests from committers that can be verified as having signed the OCA
can be accepted.

## Pull request process

1. Ensure there is an issue created to track and discuss the fix or enhancement
   you intend to submit.
1. Fork this repository.
1. Create a branch in your fork to implement the changes. We recommend using
   the issue number as part of your branch name, e.g. `1234-fixes`.
1. Ensure that any documentation is updated with the changes that are required
   by your change.
1. Ensure that any samples are updated if the base image has been changed.
1. Submit the pull request. *Do not leave the pull request blank*. Explain exactly
   what your changes are meant to do and provide simple steps on how to validate.
   your changes. Ensure that you reference the issue you created as well.
1. We will assign the pull request to 2-3 people for review before it is merged.

## Coding style
### Code format

The code must be formatted the same way. Code format is one of the validation steps of pipelines run beside merge-request
Please use the [go fmt command](https://pkg.go.dev/cmd/gofmt)
Formatting is validated in CI and contributors should run gofmt.

#### Code format in IntelliJ

[see instructions](https://www.jetbrains.com/help/idea/integration-with-go-tools.html)

### Errors

Error codes and error messages are declared in the [error_messages_en.go](./driver/common/error_messages_en.go) file. 

To create a new error:
 - declare a constant with the error code. If an ORA code exists for that error, use the ORA code; otherwise create an
 OGD error code.
 - register the error message using the [message.SetString](https://pkg.go.dev/golang.org/x/text/message#SetString) or 
 the [message.Set](https://pkg.go.dev/golang.org/x/text/message#Set) methods.

To return an error, return an instance of OracleError.
 ```
 return &OracleError{
		code:  ConnectionLost,
		cause: err,
	}
 ```
or:
 ```
 return NewOracleError(ConnectionLost, err, nil)
 ```
## Testing

### Configuration
Tests that require a database connection use a JSON file to specify the
configuration. This JSON file can contain several configurations. Each
configuration in the JSON file is identified by a name. The following flags
identify the configuration file and which configuration within the file to use:
- driver.config.filename: path to the configuration JSON file
- driver.config.name: name of the configuration to be applied at runtime within the file

The JSON file has the following format:

```
[
  {
    "config_name": "configuration_name",
    "enabled": true, // if false linked tests will be skipped
    "database_version": 26, // database version
    "driver": {
      "name": "oracledb"
    },
    "database": {
      "host": "host_name",
      "port": 1521,
      "servicename": "service_name",
      "protocol": "tcp"
    },
    "credentials": {
      "username": "username",
      "password": "password",
      "logonMode": "" // sysdba
    }
  }
]
```

### Running tests
```
go test $(go list ./...) -v  -driver.config.filename=/foo/my_config.json -driver.config.name=my_rack_in_phx 
```

### Adding tests to a test suite

Tests must belong to one or more testing categories.
Categories are free-form strings like unitary, functional, performance, and robustness. Test suites are supersets of tests and are defined in
<package name>_pkg_test.go files in Go packages.

Test suites are defined in these files as follows:

```go
var testCases = []struct {
	name       string
	categories string
	exclusive  bool
	f          func(t *testing.T)
}{
	{"test foo", "functional", false, TestFoo},
	{"test bar", "unitary", false, TestBar},
}
```
In the example above, two tests are registered: one unitary test named "test bar" that executes the TestBar test method
and one functional test named "test foo" that executes TestFoo. The exclusive field marks tests that should not run
under the parallel category executor. Tests that can be run within parallel execution (most of them should) must call
t.Parallel() at the beginning of their execution.

All <package name>_pkg_test.go files must also define a test suite executor as shown below:

```go
func TestCategoryExecutor(t *testing.T) {
  var regularCases, exclusiveCases []struct {
    name       string
    categories string
    exclusive  bool
    f          func(t *testing.T)
  }

  for _, c := range testCases {
   cats := strings.Split(c.categories, ",")
   for _, p := range cats {
     if strings.Compare(strings.TrimSpace(p), TestCategory) == 0 {
      if c.exclusive {
        exclusiveCases = append(exclusiveCases, c)
      } else {
        regularCases = append(regularCases, c)
      }
      break
     }
   }
 }

  if len(regularCases) > 0 {
    t.Run("parallel", func(t *testing.T) {
      t.Parallel()
      for _, c := range regularCases {
        t.Run(c.name, c.f)
      }
    })
  }

  for _, c := range exclusiveCases {
    t.Run(c.name, c.f)
  }
}

```
#### Tests coding rules
- Tests should remain as simple as possible and should test only one thing.
- Tests must include a documentation block that describes the test logic and expected results.

#### Running tests by category

Test suites are run by calling the "go test" command with TestCategoryExecutor as the test name filter.
The category is specified with the test.category flag as a single string or a comma-separated list of categories.
The `-parallel` flag limits parallel tests per package test binary. Use Go's `-p` flag when you need to limit how many
packages are tested at the same time.

```go
 # run all functional tests.
 go test -parallel=10  -shuffle=on  $(go list ./...) -v -run TestCategoryExecutor -test.category="functional"
```


## Code of conduct

Follow the [Golden Rule](https://en.wikipedia.org/wiki/Golden_Rule). If you'd
like more specific guidelines, see the [Contributor Covenant Code of Conduct][COC].

[OCA]: https://oca.opensource.oracle.com
[COC]: https://www.contributor-covenant.org/version/1/4/code-of-conduct/
