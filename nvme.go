// Copyright 2017 Daniel Swarbrick. All rights reserved.
// Use of this source code is governed by a GPL license that can be found in the LICENSE file.

// NVMe admin commands.

package smart

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"syscall"
	"unsafe"
)

const (
	NVME_ADMIN_GET_LOG_PAGE = 0x02
	NVME_ADMIN_IDENTIFY     = 0x06
)

var (
	NVME_IOCTL_ADMIN_CMD = _iowr('N', 0x41, unsafe.Sizeof(nvmePassthruCommand{}))
)

// Defined in <linux/nvme_ioctl.h>
type nvmePassthruCommand struct {
	opcode       uint8
	flags        uint8
	rsvd1        uint16
	nsid         uint32
	cdw2         uint32
	cdw3         uint32
	metadata     uint64
	addr         uint64
	metadata_len uint32
	data_len     uint32
	cdw10        uint32
	cdw11        uint32
	cdw12        uint32
	cdw13        uint32
	cdw14        uint32
	cdw15        uint32
	timeout_ms   uint32
	result       uint32
} // 72 bytes

type nvmeIdentPowerState struct {
	MaxPower        uint16 // Centiwatts
	Rsvd2           uint8
	Flags           uint8
	EntryLat        uint32 // Microseconds
	ExitLat         uint32 // Microseconds
	ReadTput        uint8
	ReadLat         uint8
	WriteTput       uint8
	WriteLat        uint8
	IdlePower       uint16
	IdleScale       uint8
	Rsvd19          uint8
	ActivePower     uint16
	ActiveWorkScale uint8
	Rsvd23          [9]byte
}

type nvmeIdentController struct {
	VendorID     uint16                  // PCI Vendor ID
	Ssvid        uint16                  // PCI Subsystem Vendor ID
	SerialNumber [20]byte                // Serial Number
	ModelNumber  [40]byte                // Model Number
	Firmware     [8]byte                 // Firmware Revision
	Rab          uint8                   // Recommended Arbitration Burst
	IEEE         [3]byte                 // IEEE OUI Identifier
	Cmic         uint8                   // Controller Multi-Path I/O and Namespace Sharing Capabilities
	Mdts         uint8                   // Maximum Data Transfer Size
	Cntlid       uint16                  // Controller ID
	Ver          uint32                  // Version
	Rtd3r        uint32                  // RTD3 Resume Latency
	Rtd3e        uint32                  // RTD3 Entry Latency
	Oaes         uint32                  // Optional Asynchronous Events Supported
	Rsvd96       [160]byte               // ...
	Oacs         uint16                  // Optional Admin Command Support
	Acl          uint8                   // Abort Command Limit
	Aerl         uint8                   // Asynchronous Event Request Limit
	Frmw         uint8                   // Firmware Updates
	Lpa          uint8                   // Log Page Attributes
	Elpe         uint8                   // Error Log Page Entries
	Npss         uint8                   // Number of Power States Support
	Avscc        uint8                   // Admin Vendor Specific Command Configuration
	Apsta        uint8                   // Autonomous Power State Transition Attributes
	Wctemp       uint16                  // Warning Composite Temperature Threshold
	Cctemp       uint16                  // Critical Composite Temperature Threshold
	Mtfa         uint16                  // Maximum Time for Firmware Activation
	Hmpre        uint32                  // Host Memory Buffer Preferred Size
	Hmmin        uint32                  // Host Memory Buffer Minimum Size
	Tnvmcap      [16]byte                // Total NVM Capacity
	Unvmcap      [16]byte                // Unallocated NVM Capacity
	Rpmbs        uint32                  // Replay Protected Memory Block Support
	Rsvd316      [196]byte               // ...
	Sqes         uint8                   // Submission Queue Entry Size
	Cqes         uint8                   // Completion Queue Entry Size
	Rsvd514      [2]byte                 // (defined in NVMe 1.3 spec)
	Nn           uint32                  // Number of Namespaces
	Oncs         uint16                  // Optional NVM Command Support
	Fuses        uint16                  // Fused Operation Support
	Fna          uint8                   // Format NVM Attributes
	Vwc          uint8                   // Volatile Write Cache
	Awun         uint16                  // Atomic Write Unit Normal
	Awupf        uint16                  // Atomic Write Unit Power Fail
	Nvscc        uint8                   // NVM Vendor Specific Command Configuration
	Rsvd531      uint8                   // ...
	Acwu         uint16                  // Atomic Compare & Write Unit
	Rsvd534      [2]byte                 // ...
	Sgls         uint32                  // SGL Support
	Rsvd540      [1508]byte              // ...
	Psd          [32]nvmeIdentPowerState // Power State Descriptors
	Vs           [1024]byte              // Vendor Specific
} // 4096 bytes

type nvmeLBAF struct {
	Ms uint16
	Ds uint8
	Rp uint8
}

type nvmeIdentNamespace struct {
	Nsze    uint64
	Ncap    uint64
	Nuse    uint64
	Nsfeat  uint8
	Nlbaf   uint8
	Flbas   uint8
	Mc      uint8
	Dpc     uint8
	Dps     uint8
	Nmic    uint8
	Rescap  uint8
	Fpi     uint8
	Rsvd33  uint8
	Nawun   uint16
	Nawupf  uint16
	Nacwu   uint16
	Nabsn   uint16
	Nabo    uint16
	Nabspf  uint16
	Rsvd46  [2]byte
	Nvmcap  [16]byte
	Rsvd64  [40]byte
	Nguid   [16]byte
	EUI64   [8]byte
	Lbaf    [16]nvmeLBAF
	Rsvd192 [192]byte
	Vs      [3712]byte
} // 4096 bytes

type nvmeSMARTLog struct {
	CritWarning      uint8
	Temperature      [2]uint8
	AvailSpare       uint8
	SpareThresh      uint8
	PercentUsed      uint8
	Rsvd6            [26]byte
	DataUnitsRead    [16]byte
	DataUnitsWritten [16]byte
	HostReads        [16]byte
	HostWrites       [16]byte
	CtrlBusyTime     [16]byte
	PowerCycles      [16]byte
	PowerOnHours     [16]byte
	UnsafeShutdowns  [16]byte
	MediaErrors      [16]byte
	NumErrLogEntries [16]byte
	WarningTempTime  uint32
	CritCompTime     uint32
	TempSensor       [8]uint16
	Rsvd216          [296]byte
} // 512 bytes

// WIP, highly likely to change
func OpenNVMe(dev string) error {
	fd, err := syscall.Open(dev, syscall.O_RDWR, 0600)
	if err != nil {
		return err
	}

	defer syscall.Close(fd)

	buf := make([]byte, 4096)

	cmd := nvmePassthruCommand{
		opcode:   NVME_ADMIN_IDENTIFY,
		nsid:     0, // Namespace 0, since we are identifying the controller
		addr:     uint64(uintptr(unsafe.Pointer(&buf[0]))),
		data_len: uint32(len(buf)),
		cdw10:    1, // Identify controller
	}

	if err := ioctl(uintptr(fd), NVME_IOCTL_ADMIN_CMD, uintptr(unsafe.Pointer(&cmd))); err != nil {
		return err
	}

	fmt.Printf("NVMe call: opcode=%#02x, size=%#04x, nsid=%#08x, cdw10=%#08x\n",
		cmd.opcode, cmd.data_len, cmd.nsid, cmd.cdw10)

	var controller nvmeIdentController

	binary.Read(bytes.NewBuffer(buf[:]), nativeEndian, &controller)

	fmt.Println()
	fmt.Printf("Vendor ID: %#04x\n", controller.VendorID)
	fmt.Printf("Model number: %s\n", controller.ModelNumber)
	fmt.Printf("Serial number: %s\n", controller.SerialNumber)
	fmt.Printf("Firmware version: %s\n", controller.Firmware)
	fmt.Printf("IEEE OUI identifier: 0x%02x%02x%02x\n",
		controller.IEEE[2], controller.IEEE[1], controller.IEEE[0])
	fmt.Printf("Max. data transfer size: %d pages\n", 1<<controller.Mdts)

	for _, ps := range controller.Psd {
		if ps.MaxPower > 0 {
			fmt.Printf("%+v\n", ps)
		}
	}

	buf2 := make([]byte, 4096)

	cmd = nvmePassthruCommand{
		opcode:   NVME_ADMIN_IDENTIFY,
		nsid:     1, // Namespace 1
		addr:     uint64(uintptr(unsafe.Pointer(&buf2[0]))),
		data_len: uint32(len(buf2)),
		cdw10:    0,
	}

	if err = ioctl(uintptr(fd), NVME_IOCTL_ADMIN_CMD, uintptr(unsafe.Pointer(&cmd))); err != nil {
		return err
	}

	fmt.Printf("NVMe call: opcode=%#02x, size=%#04x, nsid=%#08x, cdw10=%#08x\n",
		cmd.opcode, cmd.data_len, cmd.nsid, cmd.cdw10)

	var ns nvmeIdentNamespace

	binary.Read(bytes.NewBuffer(buf2[:]), nativeEndian, &ns)

	fmt.Printf("Namespace 1 size: %d sectors\n", ns.Nsze)
	fmt.Printf("Namespace 1 utilisation: %d sectors\n", ns.Nuse)

	buf3 := make([]byte, 512)

	// Read SMART log
	if err = readNVMeLogPage(fd, 0x02, &buf3); err != nil {
		return err
	}

	var sl nvmeSMARTLog

	binary.Read(bytes.NewBuffer(buf3[:]), nativeEndian, &sl)

	// TODO: Implement bytes to "KMGTP" function
	unitsRead := le128ToBigInt(sl.DataUnitsRead)
	unitsWritten := le128ToBigInt(sl.DataUnitsWritten)
	unit := big.NewInt(512 * 1000)

	fmt.Println("\nSMART data follows:")
	fmt.Printf("Critical warning: %#02x\n", sl.CritWarning)
	fmt.Printf("Temperature: %d Celsius\n",
		((uint16(sl.Temperature[1])<<8)|uint16(sl.Temperature[0]))-273) // Kelvin to degrees Celsius
	fmt.Printf("Avail. spare: %d%%\n", sl.AvailSpare)
	fmt.Printf("Avail. spare threshold: %d%%\n", sl.SpareThresh)
	fmt.Printf("Percentage used: %d%%\n", sl.PercentUsed)
	fmt.Printf("Data units read: %d [%d bytes]\n", unitsRead, new(big.Int).Mul(unitsRead, unit))
	fmt.Printf("Data units written: %d [%d bytes]\n", unitsWritten, new(big.Int).Mul(unitsWritten, unit))
	fmt.Printf("Host read commands: %d\n", le128ToBigInt(sl.HostReads))
	fmt.Printf("Host write commands: %d\n", le128ToBigInt(sl.HostWrites))
	fmt.Printf("Controller busy time: %d\n", le128ToBigInt(sl.CtrlBusyTime))
	fmt.Printf("Power cycles: %d\n", le128ToBigInt(sl.PowerCycles))
	fmt.Printf("Power on hours: %d\n", le128ToBigInt(sl.PowerOnHours))
	fmt.Printf("Unsafe shutdowns: %d\n", le128ToBigInt(sl.UnsafeShutdowns))
	fmt.Printf("Media & data integrity errors: %d\n", le128ToBigInt(sl.MediaErrors))
	fmt.Printf("Error information log entries: %d\n", le128ToBigInt(sl.NumErrLogEntries))

	return nil
}

// le128ToBigInt takes a little-endian 16-byte slice and returns a *big.Int representing it.
func le128ToBigInt(buf [16]byte) *big.Int {
	// Int.SetBytes() expects big-endian input, so reverse the bytes locally first
	rev := make([]byte, 16, 16)
	for x := 0; x < 16; x++ {
		rev[x] = buf[16-x-1]
	}

	return new(big.Int).SetBytes(rev)
}

func readNVMeLogPage(fd int, logID uint8, buf *[]byte) error {
	bufLen := len(*buf)

	if (bufLen < 4) || (bufLen > 0x4000) || (bufLen%4 != 0) {
		return fmt.Errorf("Invalid buffer size")
	}

	cmd := nvmePassthruCommand{
		opcode:   NVME_ADMIN_GET_LOG_PAGE,
		nsid:     0xffffffff, // FIXME
		addr:     uint64(uintptr(unsafe.Pointer(&(*buf)[0]))),
		data_len: uint32(bufLen),
		cdw10:    uint32(logID) | (((uint32(bufLen) / 4) - 1) << 16),
	}

	return ioctl(uintptr(fd), NVME_IOCTL_ADMIN_CMD, uintptr(unsafe.Pointer(&cmd)))
}
