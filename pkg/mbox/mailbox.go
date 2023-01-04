/*
Package mbox implements the Mailbox protocol used to communicate between the the VideoCore GPU and
ARM processor on a Raspberry Pi.

https://github.com/raspberrypi/firmware/wiki/Mailbox-property-interface

TODO: Thread safety
TODO: Account for different channels. Maybe in constructor?
*/
package mbox

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"github.com/cavaliercoder/rpi_export/pkg/ioctl"
)

const (
	RequestCodeDefault uint32 = 0x00000000
)

const (
	TagGetFirmwareRevision  uint32 = 0x00000001
	TagGetBoardModel        uint32 = 0x00010001
	TagGetBoardRevision     uint32 = 0x00010002
	TagGetBoardMAC          uint32 = 0x00010003
	TagGetPowerState        uint32 = 0x00020001
	TagGetClockRate         uint32 = 0x00030002
	TagGetVoltage           uint32 = 0x00030003
	TagGetMaxVoltage        uint32 = 0x00030005
	TagGetTemperature       uint32 = 0x00030006
	TagGetMinVoltage        uint32 = 0x00030008
	TagGetTurbo             uint32 = 0x00030009
	TagGetMaxTemperature    uint32 = 0x0003000A
	TagGetClockRateMeasured uint32 = 0x00030047
)

const (
	replySuccess uint32 = 0x80000000
	replyFail    uint32 = 0x80000001
)

var Debug = false

var (
	ErrNotImplemented = errors.New("vcio: not implemented")
	ErrRequestBuffer  = errors.New("vcio: error parsing request buffer")
)

var mbIoctl = ioctl.IOWR('d', 0, uint(unsafe.Sizeof(new(byte))))

type Tag []uint32

var EndTag = Tag{0}

func (t Tag) ID() uint32 {
	if !t.IsValid() {
		return 0
	}
	return t[0]
}

// Cap returns the length of the value buffer in bytes.
func (t Tag) Cap() int {
	if !t.IsValid() {
		return 0
	}
	return int(t[1])
}

// Len is the length of a response value in bytes. The actual bytes written by a response will be
// the lesser of Len and Cap. If Len is greater than Cap, retry the request with a bigger value
// buffer.
//
// To get the length of the entire tag in uint32s, including headers, use len(Tag).
func (t Tag) Len() int {
	if !t.IsValid() || !t.IsResponse() {
		return 0
	}
	return int(t[2] & 0x7FFFFFFF)
}

func (t Tag) IsResponse() bool {
	if !t.IsValid() {
		return false
	}
	return t[2]&0x80000000 == 0x80000000
}

// Value returns the value buffer. TODO: Always 32bit.
func (t Tag) Value() []uint32 {
	if !t.IsValid() {
		return nil
	}
	return t[3 : 3+t.Len()/4]
}

func (t Tag) IsEnd() bool { return len(t) == 1 && t[0] == 0 }

func (t Tag) IsValid() bool {
	if len(t) == 0 {
		return false // Nil or empty
	}
	if len(t) == 1 {
		return t.IsEnd() // End tag
	}
	if len(t) < 3 {
		return false // Too short
	}
	if len(t) != int(3+t[1]/4) {
		// Incorrect size with value buffer
		return false
	}
	return true
}

func ReadTag(b []uint32) (Tag, error) {
	if len(b) > 0 && b[0] == 0 {
		return EndTag, nil
	}
	if len(b) < 3 {
		return nil, fmt.Errorf("vcio: tag buffer is too small")
	}
	sz := 3 + int(b[1]/4) // TODO: fix unaligned buffer sizes
	if len(b) < sz {
		return nil, fmt.Errorf("vcio: tag buffer is too small")
	}
	return Tag(b[:sz]), nil
}

// Mailbox implements the Mailbox protocol used by the VideoCore and ARM on a Raspberry Pi.
type Mailbox struct {
	f            *os.File
	bufUnaligned [48]uint32
	buf          []uint32
}

func Open() (f *Mailbox, err error) {
	var ff *os.File
	ff, err = os.OpenFile("/dev/vcio", os.O_RDONLY, os.ModePerm)
	if err == os.ErrNotExist {
		return nil, ErrNotImplemented
	}
	if err != nil {
		return nil, err
	}
	return &Mailbox{f: ff}, nil
}

func (c *Mailbox) Close() (err error) {
	if c == nil || c.f == nil {
		return nil
	}
	err = c.f.Close()
	c.f = nil
	return
}

// Do sends a single command tag and returns all response tags. Returned memory is only usable until
// the next request is made.
func (m *Mailbox) Do(tagID uint32, bufferBytes int, args ...uint32) ([]Tag, error) {
	// Align buffer to 16-byte boundary
	if m.buf == nil {
		offset := uintptr(unsafe.Pointer(&m.bufUnaligned[0])) & 15
		m.buf = m.bufUnaligned[16-offset : uintptr(len(m.bufUnaligned))-offset]
	}

	// Compute value buffer length
	// TODO: ensure 32-bit aligned
	if bufferBytes < len(args)*4 {
		bufferBytes = len(args) * 4
	}

	// Write request header
	m.buf[0] = uint32(len(m.buf)) * 4 // buffer size
	m.buf[1] = RequestCodeDefault     // request code

	// Write request tag
	m.buf[2] = tagID
	m.buf[3] = uint32(bufferBytes) // Value buffer size in bytes
	m.buf[4] = 0                   // This is a request
	copy(m.buf[5:], args)          // Write value buffer
	// TODO: zero remaining buffer and end tag

	debugf("TX:\n")
	for i, v := range m.buf[:5+len(args)] {
		debugf("  %02d: 0x%08X\n", i, v)
	}

	// Send message via ioctl
	err := ioctl.Ioctl(m.f.Fd(), uintptr(mbIoctl), uintptr(unsafe.Pointer(&m.buf[0])))
	if err != nil {
		return nil, err
	}

	debugf("RX:\n")
	for i, v := range m.buf[:16] {
		debugf("  %02d: 0x%08X\n", i, v)
	}

	// Check response header
	if m.buf[1] == replyFail {
		return nil, ErrRequestBuffer
	}
	if m.buf[1]&replySuccess != replySuccess {
		return nil, fmt.Errorf("vcio: unexpected response code: 0x%08x", m.buf[1])
	}

	b := m.buf[2:]
	var tags []Tag
	for {
		t, err := ReadTag(b)
		if err != nil {
			return nil, err
		}
		if t.IsEnd() {
			break
		}
		if tags == nil {
			tags = make([]Tag, 0, 1)
		}
		tags = append(tags, t)
		b = b[len(t):]
	}

	return tags, nil
}

func (m *Mailbox) getUint32(tagID uint32) (uint32, error) {
	tags, err := m.Do(tagID, 4)
	if err != nil {
		return 0, err
	}
	// TODO: bounds check
	return tags[0].Value()[0], nil
}

func (m *Mailbox) getUint32ByID(tagID, id uint32) (uint32, error) {
	tags, err := m.Do(tagID, 8, id)
	if err != nil {
		return 0, err
	}
	// TODO: check response ID matches
	// TODO: bounds check
	return tags[0].Value()[1], nil
}

func debugf(format string, a ...interface{}) {
	if !Debug {
		return
	}
	fmt.Fprintf(os.Stderr, format, a...)
}

// GetFirmwareRevision returns the firmware revision of the VideoCore component.
func (m *Mailbox) GetFirmwareRevision() (uint32, error) {
	return m.getUint32(TagGetFirmwareRevision)
}

// GetBoardModel returns the model number of the system board.
func (m *Mailbox) GetBoardModel() (uint32, error) {
	return m.getUint32(TagGetBoardModel)
}

// GetBoardRevision returns the revision number of the system board.
func (m *Mailbox) GetBoardRevision() (uint32, error) {
	return m.getUint32(TagGetBoardRevision)
}

type PowerDeviceID uint32

const (
	PowerDeviceIDSDCard PowerDeviceID = 0x00000000
	PowerDeviceIDUART0  PowerDeviceID = 0x00000001
	PowerDeviceIDUART1  PowerDeviceID = 0x00000002
	PowerDeviceIDUSBHCD PowerDeviceID = 0x00000003
	PowerDeviceIDI2C0   PowerDeviceID = 0x00000004
	PowerDeviceIDI2C1   PowerDeviceID = 0x00000005
	PowerDeviceIDI2C2   PowerDeviceID = 0x00000006
	PowerDeviceIDSPI    PowerDeviceID = 0x00000007
	PowerDeviceIDCCP2TX PowerDeviceID = 0x00000008

// PowerDeviceIDUnknown (RPi4) PowerDeviceID = 0x00000009
// PowerDeviceIDUnknown (RPi4) PowerDeviceID = 0x0000000a
)

type PowerState uint32

const (
	PowerStateOn      uint32 = 0x00000001
	PowerStateOff     uint32 = 0x00000001
	PowerStateMissing uint32 = 0x00000010
)

func (m *Mailbox) GetPowerState(id PowerDeviceID) (PowerState, error) {
	tags, err := m.Do(TagGetPowerState, 8, uint32(id))
	if err != nil {
		return 0, err
	}
	return PowerState(tags[0].Value()[1] & 0x03), nil
}

type ClockID uint32

const (
	ClockIDEMMC     ClockID = 0x000000001
	ClockIDUART     ClockID = 0x000000002
	ClockIDARM      ClockID = 0x000000003
	ClockIDCore     ClockID = 0x000000004
	ClockIDV3D      ClockID = 0x000000005
	ClockIDH264     ClockID = 0x000000006
	ClockIDISP      ClockID = 0x000000007
	ClockIDSDRAM    ClockID = 0x000000008
	ClockIDPixel    ClockID = 0x000000009
	ClockIDPWM      ClockID = 0x00000000a
	ClockIDHEVC     ClockID = 0x00000000b
	ClockIDEMMC2    ClockID = 0x00000000c
	ClockIDM2MC     ClockID = 0x00000000d
	ClockIDPixelBVB ClockID = 0x00000000e
)

func (m *Mailbox) GetClockRate(id ClockID) (hz int, err error) {
	v, err := m.getUint32ByID(TagGetClockRate, uint32(id))
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

func (m *Mailbox) GetClockRateMeasured(id ClockID) (hz int, err error) {
	v, err := m.getUint32ByID(TagGetClockRateMeasured, uint32(id))
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

func (m *Mailbox) getTemperature(tag uint32) (float32, error) {
	v, err := m.getUint32ByID(tag, 0)
	if err != nil {
		return 0, err
	}
	return float32(v) / 1000, nil
}

// GetTemperature returns the temperature of the SoC in degrees celcius.
func (m *Mailbox) GetTemperature() (float32, error) {
	return m.getTemperature(TagGetTemperature)
}

// GetMaxTemperature returns the maximum safe temperature of the SoC in degrees celcius.
//
// Overclock may be disabled above this temperature.
func (m *Mailbox) GetMaxTemperature() (float32, error) {
	return m.getTemperature(TagGetMaxTemperature)
}

type VoltageID uint32

const (
	VoltageIDCore   VoltageID = 0x000000001
	VoltageIDSDRAMC VoltageID = 0x000000002
	VoltageIDSDRAMP VoltageID = 0x000000003
	VoltageIDSDRAMI VoltageID = 0x000000004
)

func (m *Mailbox) getVoltage(tag uint32, id VoltageID) (float32, error) {
	v, err := m.getUint32ByID(tag, uint32(id))
	if err != nil {
		return 0, err
	}
	return float32(v) / 1000000, nil
}

// GetVoltage returns the voltage of the given component.
func (m *Mailbox) GetVoltage(id VoltageID) (float32, error) {
	return m.getVoltage(TagGetVoltage, id)
}

// GetMaxVoltage returns the minimum supported voltage of the given component.
func (m *Mailbox) GetMinVoltage(id VoltageID) (float32, error) {
	return m.getVoltage(TagGetMinVoltage, id)
}

// GetMaxVoltage returns the maximum supported voltage of the given component.
func (m *Mailbox) GetMaxVoltage(id VoltageID) (float32, error) {
	return m.getVoltage(TagGetMaxVoltage, id)
}

func (m *Mailbox) GetTurbo() (bool, error) {
	tags, err := m.Do(TagGetTurbo, 8, 0)
	if err != nil {
		return false, err
	}
	return tags[0].Value()[1] == 1, nil

}
