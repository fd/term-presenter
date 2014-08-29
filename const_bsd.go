// +build darwin freebsd

package main

import "syscall"

const ioctlReadTermios = syscall.TIOCGETA
const ioctlWriteTermios = syscall.TIOCSETA
