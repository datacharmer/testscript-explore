---
title: Solving testscript problems
description: Passing data, improving setup, and creating testable commands
date: 2022-07-25
---

In the [previous post][previous], we left with the realization that something was still missing to run a full test of
[dbdeployer][dbdeployer] with [testscript][testscript]. Let's recapitulate:

* We have solved the problem of repeating scripts with minimal variation by using templates and creating the testdata scripts on-the-fly
* We have created custom commands and conditions, to make the test scripts more dynamic.
* We haven't solved the problem of initializing the testing environment with the database versions to use.
* We also haven't yet found a way of checking which versions are available before the test.
* While we were looking at the `testscript` capabilities, there are a few things that were not clear:
  * Why do we have two types of "commands"? one as parameter in `testscript.Run` and another in `testscript.RunMain`
  * How can I use a `*testing.T` method (or a testing package that embeds such type) within testscript commands?

In this article, we will check all the issues, and get away with all the solutions.

### Initializing

Preparing the server for dbdeployer operations can be done [manually][prerequisites] or using [dbdeployer itself][db-init]. 
Either way, the operations include the creation of a few directories, and the download of the latest MySQL binaries.

Running the initialization with dbdeployer would be desirable, as it also checks implicitly that the initialization
capabilities work as expected. Doing this with `testscript` would be tricky, because the regular tests need the environment
to be ready before starting. Rather than trying to solve this chicken-and-egg problem, I decided to run the initialization
using `dbdeployer` functions, instead of invoking it through `testscript`. This problem could also be solved by creating
the environment with a shell script, which works well in a CI/CD action, but I wanted this test to be runnable without
external prerequisites.

The main obstacle for this course of action was that most of `dbdeployer` command-line functions were doing their job from 
a [Cobra][cobra] signature:

```go
package cmd
// *** this is a poor solution ***
func doSomething(cmd *cobra.Command, args []string) error {
	option1, _ := cmd.Flags().GetString("option1Name")
	option2, _ := cmd.Flags().GetString("option2Name")
	res1 := something.runSomeRequest(option1)
    res2 := something.runSomeOtherRequest(option2)
	return putThingsTogheter(res1, res2)
}
```

This means that calling such function internally would be difficult, as the options for that function are passed as
command line flags. By isolating the options functions from the Cobra handling I was able at once to make the functions
more testable, and to ease the addition of more functionalities.

```go
package ops

type SomethingOptions struct {
	Option1 string
	Option2 string
}

func DoSomething(options SomethingOptions) error {
	res1 := something.runSomeRequest(options.Option1)
    res2:= something.runSomeOtherRequest(options.Option2)
	return putThingsTogheter(res1, res2)
}
```

```go
package cmd
// *** this is a better solution ***
func doSomething(cmd *cobra.Command, args []string) error {
	option1, _ := cmd.Flags().GetString("option1Name")
	option2, _ := cmd.Flags().GetString("option2Name")
	return ops.DoSomething(ops.SomethingOptions{option1, option2})
}
```

Now the [testing code][init-code] can call the initialization functionality without having to mess with the command line.
This code gets called in the `TestMain` function, before any real test starts.

### Checking available versions

The other point related to initializing the test refers to knowing which versions are available. Since we are using templates
that create a variation of each template for each MySQL version, we need to know which versions are available.
Better yet, as we may be running this test in a server that contains nothing to begin with, we need to be able to
download the MySQL binaries that we want to test with dbdeployer. 
Using a similar solution as the one seen with the initialization, we can [download each version][get-tarball-code] right 
before `TestMain` checks for available versions. This way, the test can run equally well in a place that contains many
versions already and in a place that is empty.

Thus, two problems solved, without really using `testscript`, but by improving the existing code. I would file that as a
gain and a positive side effect by trying to use `testscript`.

### Types of commands

Back to `testscript`. In the [previous post][previous], we have used custom commands passed as parameters in `testscript.Run()`.
These commands accept a list of arguments, and can be used without the initial `exec` in any script used by the current test.
Such commands work well, and I used them to my satisfaction.

There are , however, other commands that are passed as arguments to `testscript.RunMain()`. They don't (apparently) accept
parameters, and return an integer. I was in the dark about the meaning of this type of command, until I joined a Slack
channel with several experts on the topic, and I was pointed to the [gofumpt][gofumpt] project, which uses this:


```go
// in file main.go
func main() { os.Exit(main1()) }

func main1() int {
	// ...
	// do the real work
	return 0
}

// in file main_test.go
 	func TestMain(m *testing.M) {
		os.Exit(testscript.RunMain(m, map[string]func() int{
			"gofumpt": main1,
		}))
	}
```

Suddenly, everything is clear: the "gofumpt" command is the equivalent of compiling `gofumpt` and using its binary in
the `testscript` files. In fact, if we remove the custom command and place `gofumpt` in the `$PATH`, the tests run just
the same (provided that we compiled the latest version).

The advantages of this usage are:

* No need to previous steps to compile the executable. As in the case of the initialization, we can pre-compile the
  executable using a CI/CD step, but the local test would depend on it, and we may be testing the wrong version if
  the build process was not run on the current code.
* The test is guaranteed to run on the latest code.

The only tiny *disadvantage* of this approach is that, while the test result is equivalent to using the compiled executable,
we are not really testing the executable, which usually is produced by an automated process. This is not a big deal,
though, as the executable can be easily tested separately as part of a CI/CD task, just to make sure that we are not
distributing random garbage with an attached SHA256 checksum.

This is a great improvement for my tests. I made some more code improvements, and now my [main code][main-test] looks like this:

```go

func TestMain(m *testing.M) {
	// setup code here
	exitCode := testscript.RunMain(m, map[string]func() int{
		"dbdeployer": cmd.Execute,
	})
    // clean-up code here
	os.Exit(exitCode)
}
```

The main function of dbdeployer now uses the same `cmd.Execute` that is the basis for the `testscript` command "dbdeployer".

```go
func main() {
	os.Exit(cmd.Execute())
}
```

The tests that previously had lines like `exec dbdeployer [many arguments and options]` now work just as before, with the
bonus that I don't need to worry about having compiled the executable and having placed it in `$PATH`.

### Using testscript with quicktest in commands

As a final hurdle while writing my tests, I wanted to use [quicktest][quicktest] not only in the test code, but also in
the custom commands, as they can be considered, in their own, subtests.
The problem is that the signature for a custom command does not include the `*testing.T` parameter that is needed to
create a quicktest instance.

```go
func checkPorts(ts *testscript.TestScript, neg bool, args []string) {
	//...
}
```

There is a `TestScript` parameter, which allows us to run `ts.Fatalf()`, or `ts.Check()`, but we can't do the [`Assert`][assertion]
that is what makes `quicktest` desirable. 
Since the `testscript.TestScript` structure doesn't export a `*testing.T` parameter, we need to get creative.

In the call to `testscript.Run()`, where we define the custom commands, we can also call a `Setup` function

```go

func TestDbDeployer(t *testing.T) {
	if !common.DirExists("testdata") {
		t.Skip("no testdata found")
	}
	// Directories in testdata are created by the setup code in TestMain
	dirs, err := filepath.Glob("testdata/*")
	if err != nil {
		t.Skip("no directories found in testdata")
	}
	for _, dir := range dirs {
		conditionalPrint("entering TestDbDeployer/%s", dir)
		t.Run(path.Base(dir), func(t *testing.T) {
			testscript.Run(t, testscript.Params{
				Dir:       dir,
				Cmds:      customCommands(),
				Condition: customConditions,
				Setup:     dbdeployerSetup(t, dir),
			})
		})
	}
}
```
Inside the `Setup` function we do several things. One of them is to store the `*testing.T` parameter as a variable:

```go
func dbdeployerSetup(t *testing.T, dir string) func(env *testscript.Env) error {
	return func(env *testscript.Env) error {
		// more setup actions here
		env.Values["testingT"] = t

		return nil
	}
}
```
Now, the `t` argument is stored inside a variable, and the `testscript.TestScript` structure has a public `Value` field,
containing the map that was set during test preparation. With this setting in place, we can now create a `quicktest`
instance inside a command:

```go
func getTestingT(ts *testscript.TestScript) (*qt.C, error) {
	rawT := ts.Value("testingT")
	if rawT == nil {
		return nil, fmt.Errorf("error fetching T argument from setup")
	}
	t, ok := rawT.(*testing.T)
	if !ok {
		return nil, fmt.Errorf("error converting interface{} to *testing.T")
	}
	return qt.New(t), nil
}

// findErrorsInLogFile is a testscript command that finds ERROR strings inside a sandbox data directory
func findErrorsInLogFile(ts *testscript.TestScript, neg bool, args []string) {

	c, err := getTestingT(ts)
	ts.Check(err)
	if len(args) < 1 {
		ts.Fatalf("no sandbox path provided")
	}
	sbDir := args[0]
	dataDir := path.Join(sbDir, "data")
	logFile := path.Join(dataDir, "msandbox.err")
	c.Assert(common.DirExists(dataDir), qt.Equals, true)

	c.Assert(common.FileExists(logFile), qt.Equals, true)

	contents, err := ioutil.ReadFile(logFile) // #nosec G304
	ts.Check(err)
	hasError := strings.Contains(string(contents), "ERROR")
	if neg && hasError {
		ts.Fatalf("ERRORs found in %s\n", logFile)
	}
	if !neg && !hasError {
		ts.Fatalf("ERRORs not found in %s\n", logFile)
	}
}
```
In the sample above we use `getTestingT` to extract the value of `t` from the `ts` parameter. Once we make sure that
the type conversion from `interface{}` to `*testing.T` is successful, we can use such value to build a new `quicktest`
instance and use it for assertions in the rest of the  function.

### Summing up

I think my exploration of `testscript` has been successful. There is some tuning to perform, but at the end I have an
environment where I can quickly write tests and execute them easily. Compared to using shell scripts, this is a huge
technological advance. In addition to the ease of writing, the tests are also easy to read and debug.

The `testscript` library was developed with the aim of testing the `go` tool itself: if you want to test a tool that
reads and manipulates text (such as [`gofumpt`][gofumpt]), `testscript` is a perfect match. And even if, as in my case,
the text manipulation is minimal, `testscript` provides an environment that allows you to reduce boilerplate and put your
tools to the test quickly. Additionally, the ability of adding custom commands and conditions makes the environment very
powerful.

Now I *only* need to go through my large collection of shell-based tests for dbdeployer, and convert them to `testscript`.
Wish me luck.

[previous]: https://datacharmer.github.io/testscript-dbdeployer-custom-commands
[testscript]: https://pkg.go.dev/github.com/rogpeppe/go-internal@v1.8.1/testscript
[dbdeployer]: https://github.com/datacharmer/dbdeployer
[quicktest]: https://pkg.go.dev/github.com/frankban/quicktest
[gofumpt]: https://github.com/mvdan/gofumpt
[prerequisites]: https://github.com/datacharmer/dbdeployer/wiki/prerequisites
[db-init]: https://github.com/datacharmer/dbdeployer/wiki/initializing-the-environment
[init-code]: https://github.com/datacharmer/dbdeployer/blob/isolate-cmd-functions/ts_test/ts_test.go#L116
[get-tarball-code]: https://github.com/datacharmer/dbdeployer/blob/isolate-cmd-functions/ts_test/ts_test.go#L132
[cobra]: https://github.com/spf13/cobra
[main-test]: https://github.com/datacharmer/dbdeployer/blob/isolate-cmd-functions/ts_test/ts_test.go#L151
[main-code]: https://github.com/datacharmer/dbdeployer/blob/isolate-cmd-functions/main.go#L25
[assertion]: https://pkg.go.dev/github.com/frankban/quicktest#hdr-Assertions
