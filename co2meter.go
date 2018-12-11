package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"
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
	if err != nil {
		log.Fatal(err)
	}
	defer source.Close()

	for {
		select {
		case <-readTick:
			readData(source)
		case s := <-sigs:
			log.Printf("received signal: %s\n", s)
			return
		}
	}
}

func readData(source *os.File) []byte {
	// https://www.devdungeon.com/content/working-files-go#everything_is_a_file

	buffer := make([]byte, 8)
	_, err = io.ReadFull(source, buffer)
	if err != nil {
		log.Fatal(err)
	}
	// log.Printf("hex: %s\n", hex.Dump(buffer))

	decrypted := decrypt(buffer)
	// log.Printf("hex: %s", hex.Dump(decrypted))

	if decrypted[4] != 0x0d || ((decrypted[0]+decrypted[1]+decrypted[2])&0xff) != decrypted[3] {
		log.Printf("ERROR: checksum mismatch: %s => %s", hex.Dump(buffer), hex.Dump(decrypted))
	} else {
		operation := decrypted[0]
		val := ((int)(decrypted[1]) << 8) | (int)(decrypted[2])
		//self._values[operation] = val
		switch operation {
		case CO2METER_CO2:
			log.Printf("co2(ppm):%v", val)
		case CO2METER_TEMP:
			t := roundFmt(((float64)(val)/16.0 - 273.15), 0.1, "%.1f")
			log.Printf("temp(c) :%v", t)
		case CO2METER_HUM:
			log.Printf("hum (%)%v", val)
		default:
			//log.Printf("ERROR operation not recognized %x, %v", operation, val)
		}
	}

	return decrypted
}

func roundFmt(x, unit float64, format string) string {
	return fmt.Sprintf(format, math.Round(x/unit)*unit)
}

func decrypt(data []byte) []byte {
	key := []byte{0xc4, 0xc6, 0xc0, 0x92, 0x40, 0x23, 0xdc, 0x96}
	cstate := []byte{0x48, 0x74, 0x65, 0x6D, 0x70, 0x39, 0x39, 0x65}
	shuffle := []byte{2, 4, 0, 7, 1, 6, 5, 3}

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

func main() {
	// https://github.com/heinemml/CO2Meter

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	deviceName := "/dev/hidraw1"
	intervalSeconds := 1

	readLoop(deviceName, intervalSeconds)

	log.Printf("done.\n")

}
