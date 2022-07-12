---
title: Enhancing testscript tests with custom commands and conditions
description: Testing dbdeployer using testscript custom commands and conditions
date: 2022-07-11
---

In the [previous][attempt1] [posts][attempt2] we have used built-in `testscript` commands and conditions.

Some commands are `exec`, `stdout`, `exists`. These commands are convenient, but they can't do everything we need for our
testing. Fortunately, `testscript` allows users to create their own commands. There are two ways of adding custom commands:
* Using the `commands` parameter in [RunMain][runmain] within a `TestMain` function, which allows to define commands that take no arguments and return an integer
* Using the `Cmds` field in `testscript.Params`, allowing the creation of commands that can have several arguments.

It's important to consider that `testscript` commands are **assertions**: a regular execution will have no consequence,
but failing the assertion will terminate the test.

For example: `exec OS-command` will run the command and continue the test if "OS-command" exists and its execution returns
a zero exit code. If "OS-command" does not exist or its execution ends with a non-zero code, the test stops.

We have only seen one condition: `[exec:program_name]`:

```
[!exec:dbdeployer] skip 'dbdeployer executable not found'
```
A condition is a command that returns `true` or `false`. The command used after it is only executed if the condition is
true. The built-in conditions don't offer much useful material for our needs, and thus we create a few of our own, also
using a field in `testscript.Params`.

Let's start by defining a command:

```go
// findErrorsInLogFile is a testscript command that finds ERROR strings inside a sandbox data directory
func findErrorsInLogFile(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) < 1 {
		ts.Fatalf("no sandbox path provided")
	}
	sbDir := args[0]
	dataDir := path.Join(sbDir, "data")
	logFile := path.Join(dataDir, "msandbox.err")
	if !dirExists(dataDir) {
		ts.Fatalf("sandbox data dir %s not found", dataDir)
	}
	if !fileExists(logFile) {
		ts.Fatalf("file %s not found", logFile)
	}

	contents, err := ioutil.ReadFile(logFile)
	if err != nil {
		ts.Fatalf("%s", err)
	}
	hasError := strings.Contains(string(contents), "ERROR")
	if neg && hasError {
		ts.Fatalf("ERRORs found in %s\n", logFile)
	}
	if !neg && !hasError {
		ts.Fatalf("ERRORs not found in %s\n", logFile)
	}
}
```

The function `findErrorsInLogFile` examines the database log in a sandbox deployed by dbdeployer, and returns non-error
when the log contains at least one occurrence of the word "ERROR". In order to use such command, we need to give it a name
and to inform `testscript` of its existence.

```go
func TestAttempt3(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
            // find_errors will check that the error log in a sandbox contains the string ERROR
            // invoke as "find_errors /path/to/sandbox"
            // The command can be negated, i.e. it will succeed if the log does not contain the string ERROR
            // "! find_errors /path/to/sandbox"
            "find_errors": findErrorsInLogFile,
	    })
    }
}
```

In a similar way, we can create commands that perform several checks:
* Make sure that the sandbox is using as many ports as expected;
* Check that a give list of files in a directory exist;
* Sleep unconditionally a given number of seconds

You can see the full implementation on [GitHub][attempt3].

In a similar manner we can define conditions. It is another field in `testscript.Params`, and requires a function defined as
`func(condition string) (bool, error)`. The documentation is not forthcoming about what to put in such function, and by
a process of trial-and-error I was able to come out with a working example:

```go
func customConditions(condition string) (bool, error) {
	elements := strings.Split(condition, ":")
	if len(elements) == 0 {
		return false, fmt.Errorf("no condition found")
	}
	name := elements[0]
	switch name {
	case "minimum_version_for_group":
		if len(elements) < 2 {
			return false, fmt.Errorf("condition 'minimum_version_for_group' requires a version")
		}
		version := elements[1]
		if strings.HasPrefix(version, "5.7") || strings.HasPrefix(version, "8.0") {
			return true, nil
		}
		return false, nil

	case "exists_within_seconds":
		if len(elements) < 3 {
			return false, fmt.Errorf("condition 'exists_within_seconds' requires a file name and the number of seconds")
		}
		fileName := elements[1]
		delay, err := strconv.Atoi(elements[2])
		if err != nil {
			return false, err
		}
		if delay == 0 {
		    return fileExists(fileName), nil	
        }
		elapsed := 0
		for elapsed <= delay {
			time.Sleep(time.Second)
			if fileExists(fileName) {
				return true, nil
			}
			elapsed++
		}
		return false, nil

	default:
		return false, fmt.Errorf("unrecognized condition name '%s'", name)

	}
}
```

In this function we define two conditions:
* `minimum_version_for_group`, with the version passed as argument, returning true if the given version supports group replication.
* `exists_within_seconds`, with two arguments: a file name and a delay in seconds, returning true if the file exists before such delay expires.

The function accepts one string as argument, and we can treat that string however we please to get the condition name and 
its parameters, if any. In this case, I decided to follow the example of the built-in "`[exec:filename]`", where the
parameter is separated from the name by a colon (":").

These conditions can be used as shown in the sample group replication template:

```
env HOME={{.Home}}
env TMPDIR={{.TmpDir}}
env sb_dir=$HOME/sandboxes/group_msb_{{.DbPathVer}}

[!minimum_version_for_group:{{.DbVersion}}] skip 'minimum version for group replication not met'
[!exec:dbdeployer] skip 'dbdeployer executable not found'
! exists $sb_dir

exec dbdeployer deploy replication --topology=group --concurrent {{.DbVersion}}
stdout 'Group Replication directory installed in .*/sandboxes/group_msb_{{.DbPathVer}}'
stdout 'initialize_nodes'
stdout -count=5 '# Node 1'
stdout -count=3 '# Node 2'
stdout -count=3 '# Node 3'
! stderr .

exists $sb_dir
[!exists_within_seconds:$sb_dir/node3/data/msandbox.err:2] stop 'the database log for node 3 was not found within 2 seconds'

exec $sb_dir/check_nodes
stdout -count=9 'ONLINE'
! stderr .

check_file $sb_dir check_nodes exec_all_slaves metadata_all start_all sysbench_ready use_all_masters
check_file $sb_dir clear_all initialize_nodes n1 status_all test_replication use_all_slaves
check_file $sb_dir exec_all n2 replicate_from sbdescription.json stop_all test_sb_all wipe_and_restart_all
check_file $sb_dir exec_all_masters n3 restart_all send_kill_all sysbench use_all

check_file $sb_dir/node1 start stop status clear
check_file $sb_dir/node1 add_option connection.json init_db my.sandbox.cnf
check_file $sb_dir/node1 sbdescription.json show_relaylog after_start connection.sql load_grants
check_file $sb_dir/node1 replicate_from send_kill sysbench use
check_file $sb_dir/node1 metadata restart show_binlog sysbench_ready wipe_and_restart
check_file $sb_dir/node1 connection.conf grants.mysql my sb_include show_log test_sb

check_file $sb_dir/node3 start stop status clear
check_file $sb_dir/node3 add_option connection.json init_db my.sandbox.cnf
check_file $sb_dir/node3 sbdescription.json show_relaylog after_start connection.sql load_grants
check_file $sb_dir/node3 replicate_from send_kill sysbench use
check_file $sb_dir/node3 metadata restart show_binlog sysbench_ready wipe_and_restart
check_file $sb_dir/node3 connection.conf grants.mysql my sb_include show_log test_sb

check_ports $sb_dir 6

exec $HOME/sandboxes/group_msb_{{.DbPathVer}}/test_replication
stdout '# fail: 0'
! stderr .

! find_errors $sb_dir/node1
! find_errors $sb_dir/node2
! find_errors $sb_dir/node3

exec dbdeployer delete group_msb_{{.DbPathVer}}
stdout 'sandboxes/group_msb_{{.DbPathVer}}'
! stderr .
```

Before the action starts, we check that the version is among the ones that support group replication. In the resulting
testdata file, the condition would be this:

```
[!minimum_version_for_group:5.6.41] skip 'minimum version for group replication not met'
```
The test will be skipped because group replication requires 5.7. (Note: there is a more precise way of checking the
version eligibility for this feature, but for now this is enough).

Finally, this is the full code for the testing function:

```go
func TestAttempt3(t *testing.T) {
	if dryRun {
		t.Skip("Dry Run")
	}
	// Directories in testdata are created by the setup code in TestMain
	dirs, err := filepath.Glob("testdata/*")
	if err != nil {
		t.Skip("no directories found in testdata")
	}
	for _, dir := range dirs {
		t.Run(path.Base(dir), func(t *testing.T) {
			testscript.Run(t, testscript.Params{
				Dir:       dir,
				Cmds:      customCommands(),
				Condition: customConditions,
			})
		})
	}
}
```

### Summing up

We have implemented several powerful additions for our tests.
However, we still haven't addressed the problem of initializing the environment with the database versions that we want
to use in the tests. I am not sure if it can be solved, but I will try. 

 [attempt1]: https://datacharmer.prose.sh/testscript-dbdeployer-first-attempt 
 [attempt2]: https://datacharmer.prose.sh/testscript-dbdeployer-with-templates
 [runmain]: https://pkg.go.dev/github.com/rogpeppe/go-internal@v1.8.1/testscript#RunMain
 [attempt3]: https://github.com/datacharmer/testscript-explore/blob/main/attempt3/attempt3_test.go