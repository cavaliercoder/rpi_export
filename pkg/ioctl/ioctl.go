/*
Package ioctl implements the ioctl syscall for Raspberry Pi device.

Currently only Raspberry Pi OS (linux) is explicitly supported.
*/
package ioctl

/*
See also:
-  /usr/include/linux/ioctl.h
-  /usr/include/asm-generic/ioctl.h
*/

import (
	"syscall"
)

const (
	iocNrbits   uint = 8
	iocTypebits uint = 8

	iocNrshift uint = 0

	iocTypeshift = iocNrshift + iocNrbits
	iocSizeshift = iocTypeshift + iocTypebits
	iocDirshift  = iocSizeshift + iocSizebits
)

const (
	iocNone  uint = 0
	iocWrite uint = 1
	iocRead  uint = 2

	iocSizebits uint = 14
	iocDirbits  uint = 2
)

func Ioctl(fd uintptr, op uintptr, arg uintptr) error {
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, op, arg)
	if err == 0 {
		return nil
	}
	return err
}

func ioc(dir, typ, nr, size uint) uint {
	return (dir << iocDirshift) |
		(typ << iocTypeshift) |
		(nr << iocNrshift) |
		(size << iocSizeshift)
}

// IO defines an ioctl with no parameters. It corresponds to _IO in the Linux
// userland API.
func IO(typ, nr uint) uint {
	return ioc(iocNone, typ, nr, 0)
}

// IOR defines an ioctl with read (userland perspective) parameters. It
// corresponds to _IOR in the Linux userland API.
func IOR(typ, nr, size uint) uint {
	return ioc(iocRead, typ, nr, size)
}

// IOW defines an ioctl with write (userland perspective) parameters. It
// corresponds to _IOW in the Linux userland API.
func IOW(typ, nr, size uint) uint {
	return ioc(iocWrite, typ, nr, size)
}

// IOWR defines an ioctl with both read and write parameters. It corresponds to
// _IOWR in the Linux userland API.
func IOWR(typ, nr, size uint) uint {
	return ioc(iocRead|iocWrite, typ, nr, size)
}
