package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"
	// "unsafe"
)

var (
	fileInfo os.FileInfo
	err      error
)

const (
	CO2METER_CO2     = 0x50
	CO2METER_TEMP    = 0x42
	CO2METER_HUM     = 0x44
	HIDIOCSFEATURE_9 = 0xC0094806
)

func readLoop(deviceName string, periodSeconds int) {

	readTick := time.Tick(time.Duration(periodSeconds) * time.Second)

	// https://www.reddit.com/r/golang/comments/4hktbe/read_user_input_until_he_press_ctrlc/
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	source, err := os.Open(deviceName)
	// source, err := os.OpenFile(deviceName, os.O_APPEND, 0660)
	//source, err := os.OpenFile(deviceName, os.O_RDWR, 0660)
	if err != nil {
		log.Fatal(err)
	}

	// log.Printf("before ioctl %v", key)
	// // https://gist.github.com/tetsu-koba/33b339d26ac9c730fb09773acf39eac5
	// _, _, errno := syscall.Syscall(
	// 	syscall.SYS_IOCTL,
	// 	uintptr(source.Fd()),
	// 	uintptr(HIDIOCSFEATURE_9),
	// 	uintptr(unsafe.Pointer(&key)),
	// )
	// if errno != 0 {
	// 	log.Printf("%v", errno)
	// }
	// log.Printf("after  ioctl %v", key)

	defer source.Close()

	for {
		select {
		case <-readTick:
			fmt.Println(string(readMeasurements(source).json()))
		case s := <-sigs:
			log.Printf("received signal: %s\n", s)
			return
		}
	}
}

func readData(source *os.File) *Measurements {
	// https://www.devdungeon.com/content/working-files-go#everything_is_a_file

	buffer := make([]byte, 8)
	_, err = io.ReadFull(source, buffer)
	if err != nil {
		log.Fatal(err)
	}

	decrypted := decrypt(buffer)
	result := parseData(decrypted)
	return result
}

func isChecksumOk(decrypted []byte) bool {
	if decrypted[4] != 0x0d || ((decrypted[0]+decrypted[1]+decrypted[2])&0xff) != decrypted[3] {
		return true
	}
	return false
}

type Measurements struct {
	Temperature string `json:"temp"`
	Co2         string `json:"co2"`
}

func (m *Measurements) json() []byte {
	bytes, err := json.Marshal(m)
	if err != nil {
		log.Println(err)
	}
	return bytes
}

func (m *Measurements) hasTemp() bool {
	if len(m.Temperature) > 0 {
		return true
	}
	return false
}

func (m *Measurements) hasCo2() bool {
	if len(m.Co2) > 0 {
		return true
	}
	return false
}

func (m *Measurements) updateWith(src *Measurements) *Measurements {
	if src.hasTemp() {
		m.Temperature = src.Temperature
	}
	if src.hasCo2() {
		m.Co2 = src.Co2
	}
	return m
}

func parseData(decrypted []byte) *Measurements {
	result := Measurements{
		Temperature: "",
		Co2:         "",
	}
	if isChecksumOk(decrypted) {
		log.Printf("ERROR: checksum mismatch: %s", hex.Dump(decrypted))
	} else {
		operation := decrypted[0]
		val := ((int)(decrypted[1]) << 8) | (int)(decrypted[2])
		//self._values[operation] = val
		switch operation {
		case CO2METER_CO2:
			result.Co2 = fmt.Sprintf("%v", val)
		case CO2METER_TEMP:
			result.Temperature = roundFmt(((float64)(val)/16.0 - 273.15), 0.1, "%.1f")
		case CO2METER_HUM:
			log.Printf("hum (%)%v", val)
		default:
			// log.Printf("ERROR operation not recognized %x, %v", operation, val)
		}
	}
	return &result
}

func readMeasurements(source *os.File) *Measurements {
	result := Measurements{
		Temperature: "",
		Co2:         "",
	}
	for !(result.hasTemp() && result.hasCo2()) {
		result.updateWith(readData(source))
	}
	// fmt.Printf("%v\n", result)
	return &result
}

func round(x, unit float64) float64 {
	return math.Round(x/unit) * unit
}

func roundFmt(x, unit float64, format string) string {
	return fmt.Sprintf(format, round(x, unit))
}

func decrypt(data []byte) []byte {
	var key = []byte{0xc4, 0xc6, 0xc0, 0x92, 0x40, 0x23, 0xdc, 0x96}
	var cstate = []byte{0x48, 0x74, 0x65, 0x6D, 0x70, 0x39, 0x39, 0x65}
	var shuffle = []byte{2, 4, 0, 7, 1, 6, 5, 3}

	phase1 := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	for i, j := range shuffle {
		phase1[j] = data[i]
	}

	phase2 := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	for i := range shuffle {
		phase2[i] = phase1[i] ^ key[i]
	}

	phase3 := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	for i := range shuffle {
		phase3[i] = ((phase2[i] >> 3) | (phase2[(i-1+8)%8] << 5)) & 0xff
	}

	ctmp := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	for i := range shuffle {
		ctmp[i] = ((cstate[i] >> 4) | (cstate[i] << 4)) & 0xff
	}

	out := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	for i := range shuffle {
		out[i] = (byte)(((0x100 + (int)(phase3[i]) - (int)(ctmp[i])) & (int)(0xff)))
	}
	return out
}

func deviceName() string {
	if len(os.Args) <= 1 {
		log.Fatal("ERROR: device name not specified\nUSAGE: co2meter /dev/hidraw[0-9]")
	}
	return os.Args[1]
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	deviceName := deviceName()
	intervalSeconds := 5
	readLoop(deviceName, intervalSeconds)
}
