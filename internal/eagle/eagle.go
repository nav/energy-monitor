// Package eagle decodes push payloads from a Rainforest EAGLE (RFA-Z109) energy
// monitor's Uploader feature, per Rainforest's Uploader API Manual v6.
package eagle

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Rainforest is the root element the EAGLE wraps every uploaded fragment in.
type Rainforest struct {
	XMLName                   xml.Name                   `xml:"rainforest"`
	MacId                     string                     `xml:"macId,attr"`
	Timestamp                 string                     `xml:"timestamp,attr"`
	InstantaneousDemand       *InstantaneousDemand       `xml:"InstantaneousDemand"`
	CurrentSummationDelivered *CurrentSummationDelivered `xml:"CurrentSummationDelivered"`
	PriceCluster              *PriceCluster              `xml:"PriceCluster"`
}

// Parse decodes a raw EAGLE upload body. The device sends `Content-Type:
// application/x-www-form-urlencoded` even though the body is XML, so callers should pass
// the raw body bytes here rather than parsing it as a form.
func Parse(body []byte) (*Rainforest, error) {
	var rf Rainforest
	if err := xml.Unmarshal(body, &rf); err != nil {
		return nil, fmt.Errorf("decoding rainforest payload: %w", err)
	}
	return &rf, nil
}

// InstantaneousDemand is the current consumption rate as recorded by the meter.
type InstantaneousDemand struct {
	DeviceMacId string `xml:"DeviceMacId"`
	MeterMacId  string `xml:"MeterMacId"`
	TimeStamp   string `xml:"TimeStamp"`
	Demand      string `xml:"Demand"`
	Multiplier  string `xml:"Multiplier"`
	Divisor     string `xml:"Divisor"`
}

// KW decodes the demand reading, in kilowatts: value = raw * multiplier / divisor, per
// the manual's own worked example ("5944 x 1 / 1000 = 5.944 kWh", i.e. kW - the manual's
// unit label there is a typo, since demand is a power reading, not an energy one). A
// multiplier or divisor of 0 is treated as 1. Demand is a 24-bit signed integer.
func (d *InstantaneousDemand) KW() (float64, error) {
	raw, err := parseSigned24Hex(d.Demand)
	if err != nil {
		return 0, fmt.Errorf("demand: %w", err)
	}
	mult, err := parseHexOrOne(d.Multiplier)
	if err != nil {
		return 0, fmt.Errorf("multiplier: %w", err)
	}
	div, err := parseHexOrOne(d.Divisor)
	if err != nil {
		return 0, fmt.Errorf("divisor: %w", err)
	}
	return float64(raw) * mult / div, nil
}

// CurrentSummationDelivered is the total consumption to date as recorded by the meter.
type CurrentSummationDelivered struct {
	DeviceMacId        string `xml:"DeviceMacId"`
	MeterMacId         string `xml:"MeterMacId"`
	TimeStamp          string `xml:"TimeStamp"`
	SummationDelivered string `xml:"SummationDelivered"`
	SummationReceived  string `xml:"SummationReceived"`
	Multiplier         string `xml:"Multiplier"`
	Divisor            string `xml:"Divisor"`
}

// DeliveredKWh decodes the cumulative commodity delivered from the utility to the user.
func (s *CurrentSummationDelivered) DeliveredKWh() (float64, error) {
	return s.decode(s.SummationDelivered)
}

// ReceivedKWh decodes the cumulative commodity received from the user by the utility
// (e.g. solar export).
func (s *CurrentSummationDelivered) ReceivedKWh() (float64, error) {
	return s.decode(s.SummationReceived)
}

func (s *CurrentSummationDelivered) decode(rawHex string) (float64, error) {
	raw, err := parseHexUint(rawHex)
	if err != nil {
		return 0, fmt.Errorf("summation: %w", err)
	}
	mult, err := parseHexOrOne(s.Multiplier)
	if err != nil {
		return 0, fmt.Errorf("multiplier: %w", err)
	}
	div, err := parseHexOrOne(s.Divisor)
	if err != nil {
		return 0, fmt.Errorf("divisor: %w", err)
	}
	return float64(raw) * mult / div, nil
}

// PriceCluster is the current price in effect on the meter (or a user-defined price set
// directly on the EAGLE).
type PriceCluster struct {
	DeviceMacId    string `xml:"DeviceMacId"`
	MeterMacId     string `xml:"MeterMacId"`
	Price          string `xml:"Price"`
	Currency       string `xml:"Currency"`
	TrailingDigits string `xml:"TrailingDigits"`
	Tier           string `xml:"Tier"`
	RateLabel      string `xml:"RateLabel"`
}

// isoCurrencyNames maps a subset of ISO 4217 numeric currency codes to their common
// three-letter names, for display purposes only.
var isoCurrencyNames = map[uint64]string{
	840: "USD",
	124: "CAD",
	978: "EUR",
	826: "GBP",
	036: "AUD",
}

// PricePerUnit decodes the current price: value = raw / 10^TrailingDigits.
func (p *PriceCluster) PricePerUnit() (float64, error) {
	raw, err := parseHexUint(p.Price)
	if err != nil {
		return 0, fmt.Errorf("price: %w", err)
	}
	digits, err := parseHexUint(p.TrailingDigits)
	if err != nil {
		return 0, fmt.Errorf("trailingDigits: %w", err)
	}
	return float64(raw) / math.Pow(10, float64(digits)), nil
}

// CurrencyName returns the common three-letter name for the ISO 4217 numeric currency
// code, or the raw numeric code as a string if it isn't one of the common currencies this
// package recognizes.
func (p *PriceCluster) CurrencyName() string {
	code, err := parseHexUint(p.Currency)
	if err != nil {
		return p.Currency
	}
	if name, ok := isoCurrencyNames[code]; ok {
		return name
	}
	return strconv.FormatUint(code, 10)
}

func parseHexUint(s string) (uint64, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if s == "" {
		return 0, fmt.Errorf("empty hex value")
	}
	return strconv.ParseUint(s, 16, 64)
}

// parseSigned24Hex parses a hex string as a 24-bit signed (two's complement) integer, as
// used for the Demand field.
func parseSigned24Hex(s string) (int64, error) {
	v, err := parseHexUint(s)
	if err != nil {
		return 0, err
	}
	if v&0x800000 != 0 {
		v -= 0x1000000
	}
	return int64(v), nil
}

// parseHexOrOne parses a hex string, treating a value of 0 as 1 (per the manual's rule
// for Multiplier/Divisor fields).
func parseHexOrOne(s string) (float64, error) {
	v, err := parseHexUint(s)
	if err != nil {
		return 0, err
	}
	if v == 0 {
		return 1, nil
	}
	return float64(v), nil
}
