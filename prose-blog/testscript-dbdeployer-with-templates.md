---
title: Using testscript with dbdeployer and templates
description: Generating several testdata elements from a template
date: 2022-07-10
---

In the [previous post](https://datacharmer.prose.sh/testscript-dbdeployer-first-attempt) we saw several problems with 
testscript usage. In this post we'll focus on these three problems:

* HOME directory and TMPDIR need to be customized
* MySQL version needs to be repeated for each script
* The `stdout` regular expression cannot contain variables.

The solution for all the above is to have dynamically generated scripts in `testdata`. We will start with a template for
a single deployment, which we place in the `templates` directory.

```
# templates/single.tmpl
env HOME={{.Home}}
env TMPDIR={{.TmpDir}}
env sb_dir=$HOME/sandboxes/msb_{{.DbPathVer}}

[!exec:dbdeployer] skip 'dbdeployer executable not found'

! exists $sb_dir

exec dbdeployer deploy single {{.DbVersion}}
stdout 'Database installed in .*/sandboxes/msb_{{.DbPathVer}}'
stdout 'sandbox server started'
! stderr .
exists $sb_dir

exists $sb_dir/use
exists $sb_dir/start
exists $sb_dir/status
exists $sb_dir/stop

exec $sb_dir/test_sb
stdout '# fail  :     0'
! stderr .

exec dbdeployer delete msb_{{.DbPathVer}}
stdout 'sandboxes/msb_{{.DbPathVer}}'
! stderr .
! exists $sb_dir
```

This template contains the same statements used in the previous posts, with the difference that the literal values are now
set as [text/template](https://pkg.go.dev/text/template) variables. Several advantages are evident here:

* The HOME directory and TMPDIR are set programmatically
* The database version and its corresponding directory name are also defined dynamically, and we can use them in the `stdout` expected text.

Now, to the practicalities. A test function using `testcript` will fail if the test data directory doesn't exist or it is empty.
Thus, we need to fill that directory before the testing function starts. We do that using [TestMain](https://pkg.go.dev/testing#hdr-Main).

```go
var dryRun bool

func TestMain(m *testing.M) {
	flag.BoolVar(&dryRun, "dry", false, "creates testdata without running tests")
	versions := []string{"5.6.41", "5.7.30", "8.0.29"}

	for _, v := range versions {
		label := strings.Replace(v, ".", "_", -1)
		err := buildTests("templates", "testdata", label, map[string]string{
			"DbVersion": v,
			"DbPathVer": label,
			"Home":      os.Getenv("HOME"),
			"TmpDir":    "/tmp",
		})
		if err != nil {
			fmt.Printf("error creating the tests for %s :%s\n", label, err)
			os.Exit(1)
		}
	}
	exitCode := m.Run()

	if dirExists("testdata") && !dryRun {
		_ = os.RemoveAll("testdata")
	}
	os.Exit(exitCode)
}
```

When `TestMain` exists, it gets called instead of any individual tests, so that it can perform any setup that is needed
before the tests start. It can then clean up after the test ends.
In the function above, we call `buildTests` for each of the database versions we want to use. For each version `x.x.xx`,
it will create a directory `x_x_xx` containing a file `single_x_x_xx.txt`. In the `templates` directory we have three
files (single.tmpl, multiple.tmpl, replication.tmpl), and therefore the testdata directory will look like this:

```
$ tree -A testdata
testdata
├── 5_6_41
│   ├── multiple_5_6_41.txt
│   ├── replication_5_6_41.txt
│   └── single_5_6_41.txt
├── 5_7_30
│   ├── multiple_5_7_30.txt
│   ├── replication_5_7_30.txt
│   └── single_5_7_30.txt
└── 8_0_29
    ├── multiple_8_0_29.txt
    ├── replication_8_0_29.txt
    └── single_8_0_29.txt
```

The `buildTests` function is not relevant for this post. You can see the details in [GitHub](https://github.com/datacharmer/testscript-explore/blob/main/attempt2/attempt2_test.go).

The test function itself, instead, needs some adjustment compared to what we've seen in the previous posts.

```go
func TestAttempt2(t *testing.T) {
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
				Dir: dir,
			})
		})
	}
}
```

You have seen that we defined a `dryRun` variable. When we run the tests with option `-dry`, it will create testdata but
will skip the test execution proper. This way you can inspect the testdata files to see whether they were created to your 
satisfaction.

Rather than using files in `testdata`, the test looks for subdirectories under it, and runs a subtest for each one. In
our case, it will run `TestAttempt2/5_6_41`, `TestAttempt2/5_7_30`, and `TestAttempt2/8_0_29`.

Now, we have solved three problems, but we have introduced a new one:

```
$ go test -v
=== RUN   TestAttempt2
=== RUN   TestAttempt2/5_6_41
=== RUN   TestAttempt2/5_6_41/multiple_5_6_41
=== PAUSE TestAttempt2/5_6_41/multiple_5_6_41
=== RUN   TestAttempt2/5_6_41/replication_5_6_41
=== PAUSE TestAttempt2/5_6_41/replication_5_6_41
=== RUN   TestAttempt2/5_6_41/single_5_6_41
=== PAUSE TestAttempt2/5_6_41/single_5_6_41
=== CONT  TestAttempt2/5_6_41/multiple_5_6_41
=== CONT  TestAttempt2/5_6_41/replication_5_6_41
=== CONT  TestAttempt2/5_6_41/single_5_6_41
[...]
=== RUN   TestAttempt2/5_7_30
=== RUN   TestAttempt2/5_7_30/multiple_5_7_30
=== PAUSE TestAttempt2/5_7_30/multiple_5_7_30
=== RUN   TestAttempt2/5_7_30/replication_5_7_30
=== PAUSE TestAttempt2/5_7_30/replication_5_7_30
=== RUN   TestAttempt2/5_7_30/single_5_7_30
=== PAUSE TestAttempt2/5_7_30/single_5_7_30
=== CONT  TestAttempt2/5_7_30/multiple_5_7_30
=== CONT  TestAttempt2/5_7_30/single_5_7_30
=== CONT  TestAttempt2/5_7_30/replication_5_7_30
=== CONT  TestAttempt2/5_7_30/single_5_7_30
[...]
=== RUN   TestAttempt2/8_0_29
=== RUN   TestAttempt2/8_0_29/multiple_8_0_29
=== PAUSE TestAttempt2/8_0_29/multiple_8_0_29
=== RUN   TestAttempt2/8_0_29/replication_8_0_29
=== PAUSE TestAttempt2/8_0_29/replication_8_0_29
=== RUN   TestAttempt2/8_0_29/single_8_0_29
=== PAUSE TestAttempt2/8_0_29/single_8_0_29
=== CONT  TestAttempt2/8_0_29/multiple_8_0_29
=== CONT  TestAttempt2/8_0_29/single_8_0_29
=== CONT  TestAttempt2/8_0_29/replication_8_0_29
```

All tests, in all subdirectories, ara running in parallel. In some situations, this could be desirable, but when dealing
with I/O heavy tests, where each run creates one or more instances of a database server and then performs data operations
on them, the limits of the host would quickly become evident.
Here we only have three templates and three database versions, but in the real test there will be a dozen versions and
possibly hundreds of templates, although not all of them apply to all the versions.
Given this situation, it would be advisable to split the load of the testing scripts across several test functions, and
if even that creates more load than the host can handle at once, split the tests across several packages.

If your host is not a beefy one, you can disable parallelization:

```
$ go test -parallel 1
```

However, this will only prevent subtests from running in parallel, but not `testscript` from running all the scripts in a
test data directory at once.

### Summing up

In this post we have overcome three big limitations that were found in the previous attempts. There is more that can be
done, though. For example:

* Add specific tests that check for errors in the database logs of every sandbox installed;
* Add tests that make sure the database is using the number of ports that are expected for that version;
* Simplify the file check, which can become quite long for complex topologies.

In the next posts we will try to address these points.