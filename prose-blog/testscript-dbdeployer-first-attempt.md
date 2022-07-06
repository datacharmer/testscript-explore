---
title: First attempt at testscript with dbdeployer
description: How I created a functioning test, overcoming the first problems
date: 2022-07-06
---

# Getting testscript to do the heavy lifting

My main reason for exploring [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal@v1.8.1/testscript) is that I want a framework that allows me to test compiled applications reliably, within regular Go testing files. The app to which I want to apply this experience is [dbdeployer](https://www.dbdeployer.com), a tool that allows the deployment of MySQL databases in sandboxes, either standalone or in replication.
My current tests (and there are a lot of them) run using a set of complex shell scripts, which, as often happens, have become hard to maintain, and sometimes fail for no detectable reason.

Here's my first shot at testing dbdeployer with testscript. The Go file is identical to the one used in the [intro](https://github.com/datacharmer/testscript-explore/tree/main/intro). The difference is in the text file (code available on [GitHub](https://github.com/datacharmer/testscript-explore/tree/main/attempt1)):

```
env HOME=/Users/gmax
env TMPDIR=/tmp
env version=8.0.29
env sb_name=msb_8_0_29

[!exec:dbdeployer] skip 'dbdeployer executable not found'
exists $HOME/opt/mysql/$version
! exists $HOME/sandboxes/$sb_name

exec dbdeployer deploy single $version
stdout 'Database installed in .*/sandboxes/'
stdout 'sandbox server started'
! stderr .

exists $HOME/sandboxes/$sb_name

exec dbdeployer delete $sb_name
stdout 'sandboxes/msb_'
! stderr .

! exists $HOME/sandboxes/$sb_name
```

Running this test will have dbdeployer deploy a single sandbox of MySQL 8.0.29, check that the execution produces the expected result, and then remove the sandbox.

This test works (on **my computer**)  but there are several problems that I need to outline. Let's go line by line:

```
env HOME=/Users/gmax
```

When testscript runs, it sets several environment variables, the most notable fo which is `HOME=no-home`. Since `dbdeployer` requires a home directory to run, the execution fails immediately.
There are ways of setting a fake HOME that satisfies `dbdeployer`, but for now I use this workaround. This will work on my Mac, but fail on Linux. I have found a better workaround, but we'll see it in another post.

```
env TMPDIR=/tmp
```

The default for this value is `$WORK/.tmp`, and `$WORK` is a subdirectory of the temporary directory. Unfortunately, in MacOS the temporary directory is something like `/private/var/folders/rz/cn7hvgzd1dl5y23l378dsf_c0000gn/T/`, to which testscript adds its own temporary directory (one for each text file), resulting in a file name that is longer than 103 characters, which happens to be the maximum available to MySQL servers. The deployment fails with this error:

```
2022-07-06T18:30:09.246639Z 0 [ERROR] [MY-010267] [Server] The socket file path is too long (> 103): /private/var/folders/rz/cn7hvgzd1dl5y23l378dsf_c0000gn/T/go-test-script2140328262/script-attempt1/.tmp/mysql_sandbox8029.sock
```
There is probably something that `dbdeployer` can do to alleviate this situation, but for the moment we use this quick workaround.

```
env version=8.0.29
```

This is the version of MySQL that we want to install. Of course, we need to have binaries for such version where we run the tests. Also, when we test the same behavior for different versions, we don't want to repeat the whole script changing only the version.
In the shell scripts, both problems are easily addressed: `dbdeployer` can detect which versions are available, and it can also download the version we need. The testscript syntax can't match the capabilities of the Bash shell, and we probably don't want it to happen. More workaround will come in the next posts.


```
env sb_name=msb_8_0_29
```
This is a simple transformation of the version number to create the sandbox name. Shell scripts can do it, but testscript needs to be provided with the full string.

```
[!exec:dbdeployer] skip 'dbdeployer executable not found'
```

This is a check that the executable we are testing exists in `$PATH`. It is also a method of skipping this test if we run the test in a host that was not initialized.

```
exists $HOME/opt/mysql/$version
```

We assert that the required version exists in this host. The test fails here if this is not true.

```
! exists $HOME/sandboxes/$sb_name
```

Similarly, we make sure that the database sandbox does not exist.

```
exec dbdeployer deploy single $version
```

This is the main command, which creates a database sandbox of the required version. If this command returns a non-zero exit code, the test fails.

```
stdout 'Database installed in .*/sandboxes/'
stdout 'sandbox server started'
! stderr .
exists $HOME/sandboxes/$sb_name
```
After the command, we run four assertions. The first two want to see something specific in the standard output. The third one says that the execution should produce no standard error. The last one checks that the directory we expect (which we checked earlier that did not exist) now is there.


```
exec dbdeployer delete $sb_name
stdout 'sandboxes/msb_'
! stderr .
```

Deleting the sandbox works by running `dbdeployer` with the appropriate parameters. We check that the output contains just a portion of the sandbox name. Unfortunately, we can't use variables in the `stdout` parameters.

```
! exists $HOME/sandboxes/$sb_name
```

As the last operation, we check that the sandbox directory has been removed.

## Problems found so far

Here is a recap of the issues that we have seen so far. We will try overcome all of them in the next posts

* Home directory needs to be customized
* TMPDIR needs to be customized
* MySQL version needs to be repeated for each script
* We need to make sure that the binaries for such version are available.
* The executable that we are testing should be available because it is being created before the test starts. This can happen easily with a CI/CD action, but it is not so easy to apply the same logic in the local machine. We risk testing an earlier version.
* The `stdout` regular expression cannot contain variables.
