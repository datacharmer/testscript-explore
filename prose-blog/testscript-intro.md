---
title: Intro to testscript usage in Go
description: Some introductory notes on how to use testscript in Go tests
date: 2022-07-06
---

# Notes on testscript usage

I've started exploring [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal@v1.8.1/testscript), which, according to the docs, "provides support for defining filesystem-based tests by creating scripts in a directory".

It's an interesting paradigm, because greatly simplifies the testing of compiled applications, rather than functions from the code. I have been searching for a framework that allows writing Go tests for tools without using shell scripts.

The basic functioning requires three steps:

1. Create a directory (say, `testdata`) containing one or more `*.txt` files, which are the testscript tests.
2. Add `import "github.com/rogpeppe/go-internal/testscript"` to your Go test code.
3. Write a test function that uses the test files:

```go
func TestFoo(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
	})
}
```

The actual test file could be something like the following: (taken from the docs)

```
# hello world
exec cat hello.text
stdout 'hello world\n'
! stderr .

-- hello.text --
hello world
```

Several interesting points here should be mentioned.
* The first one is in the last two lines: you can create a file by indicating a file name delimited by two "`--`". The test framework will create the file before executing anything.
* Second, we can execute commands using `exec` (unlike the shell `exec`, it does not end the script).
* Third, we can state assertions on the outcome of the command. For example "`stdout 'hello world\n'`" defines the fill text that we expect from running the command. We don't need to be so literal, though. The keywork `stdout` accepts regular expressions. I could as well have written "`stdout 'hel.*ld'`" or "`stdout 'h[a-z]+ w[a-z]+'`" and it would have succeeded.
* Fourth, an assertion could also be negated, like in "`! stderr .`", which means that we don't expect the standard error to produce anything.


The testing of a more complex application needs some planning, and it also involves subtle challenges. But for now, I just wanted to point out the existence of this handy testing library.
In the next days I will show some examples of how `testscript` can be used to write powerful tests with a few lines of code, and how I have overcome the first obstacles.

The code used in this post is available in the [testscript-explore repository](https://github.com/datacharmer/testscript-explore/tree/main/intro).
