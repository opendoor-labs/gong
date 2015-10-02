package pwm

import (
	"fmt"
	"math"
	"time"

	"github.com/kidoman/embd"
	_ "github.com/kidoman/embd/host/rpi" // This loads the RPi driver
)

const (
	// Registers/etc.
	Mode1         = 0x00
	Mode2         = 0x01
	SubAdr1       = 0x02
	SubAdr2       = 0x03
	SubAdr3       = 0x04
	Prescale      = 0xFE
	LED0_On_L     = 0x06
	LED0_On_H     = 0x07
	LED0_Off_L    = 0x08
	LED0_Off_H    = 0x09
	All_LED_On_L  = 0xFA
	All_LED_On_H  = 0xFB
	All_LED_Off_L = 0xFC
	All_LED_Off_H = 0xFD

	// Bits
	Restart  = 0x80
	Sleep    = 0x10
	AllCall  = 0x01
	Invert   = 0x10
	Outdrive = 0x04
)

func Init() error {
	return embd.InitI2C()
}

func Close() error {
	return embd.CloseI2C()
}

type Device struct {
	addr  byte
	bus   embd.I2CBus
	debug bool
}

func New(devNum, i2cAddr byte) (*Device, error) {
	dev := &Device{
		addr:  i2cAddr,
		bus:   embd.NewI2CBus(devNum),
		debug: true,
	}
	if err := dev.SetAllPWM(0, 0); err != nil {
		dev.Close()
		return nil, err
	}

	if err := dev.bus.WriteByte(Mode2, Outdrive); err != nil {
		dev.Close()
		return nil, err
	}
	if err := dev.bus.WriteByte(Mode1, AllCall); err != nil {
		dev.Close()
		return nil, err
	}
	time.Sleep(5 * time.Millisecond) // wait for oscillator

	mode1, err := dev.bus.ReadByte(Mode1)
	if err != nil {
		dev.Close()
		return nil, err
	}
	mode1 = mode1 &^ Sleep // wake up (reset sleep)
	if err = dev.bus.WriteByte(Mode1, mode1); err != nil {
		dev.Close()
		return nil, err
	}
	time.Sleep(5 * time.Millisecond) // wait for oscillator

	// Set frequency to 60 Hz
	if err = dev.SetPWMFreq(60); err != nil {
		dev.Close()
		return nil, err
	}
	return dev, nil
}

func (d *Device) Close() error {
	return d.bus.Close()
}

// SetAllPWM sets a PWM state on all PWM channels.
func (d *Device) SetAllPWM(on, off uint16) error {
	if err := d.bus.WriteByte(All_LED_On_L, byte(on&0xFF)); err != nil {
		return err
	}
	if err := d.bus.WriteByte(All_LED_On_H, byte(on>>8)); err != nil {
		return err
	}
	if err := d.bus.WriteByte(All_LED_Off_L, byte(off&0xFF)); err != nil {
		return err
	}
	if err := d.bus.WriteByte(All_LED_Off_H, byte(off>>8)); err != nil {
		return err
	}
	return nil
}

// SetPWM sets a PWM state on a single PWM channel.
func (d *Device) SetPWM(channel byte, on, off uint16) error {
	if err := d.bus.WriteByte(LED0_On_L+4*channel, byte(on&0xFF)); err != nil {
		return err
	}
	if err := d.bus.WriteByte(LED0_On_H+4*channel, byte(on>>8)); err != nil {
		return err
	}
	if err := d.bus.WriteByte(LED0_Off_L+4*channel, byte(off&0xFF)); err != nil {
		return err
	}
	if err := d.bus.WriteByte(LED0_Off_H+4*channel, byte(off>>8)); err != nil {
		return err
	}
	return nil
}

// SetPWMFreq sets the PWM frequency.
func (d *Device) SetPWMFreq(hz uint16) error {
	prescaleval := float64(25000000) // 25MHz
	prescaleval /= 4096              // 12-bit
	prescaleval /= float64(hz)
	prescaleval -= 1
	if d.debug {
		fmt.Printf("Setting PWM frequency to %d Hz\n", hz)
		fmt.Printf("Estimated pre-scale: %d\n", prescaleval)
	}
	prescale := math.Floor(prescaleval + 0.5)
	if d.debug {
		fmt.Printf("Final pre-scale: %d\n", prescale)
	}

	oldmode, err := d.bus.ReadByte(Mode1)
	if err != nil {
		return err
	}
	newmode := (oldmode & 0x7F) | 0x10 // sleep
	// go to sleep
	if err := d.bus.WriteByte(Mode1, newmode); err != nil {
		return err
	}
	if err := d.bus.WriteByte(Prescale, byte(math.Floor(prescale))); err != nil {
		return err
	}
	if err := d.bus.WriteByte(Mode1, oldmode); err != nil {
		return err
	}
	time.Sleep(5 * time.Millisecond)
	if err := d.bus.WriteByte(Mode1, oldmode|0x80); err != nil {
		return err
	}
	return nil
}
