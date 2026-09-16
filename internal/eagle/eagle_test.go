package eagle

import "testing"

// Fixture taken verbatim from Rainforest's Uploader API Manual v6, section
// "Data Structures > 1. Requests > Example".
const instantaneousDemandFixture = `<?xml version="1.0"?>
<rainforest macId="0xf0ad4e00ce69" timestamp="1355292588s">
<InstantaneousDemand>
<DeviceMacId>0x00158d0000000004</DeviceMacId>
<MeterMacId>0x00178d0000000004</MeterMacId>
<TimeStamp>0x185adc1d</TimeStamp>
<Demand>0x001738</Demand>
<Multiplier>0x00000001</Multiplier>
<Divisor>0x000003e8</Divisor>
<DigitsRight>0x03</DigitsRight>
<DigitsLeft>0x00</DigitsLeft>
<SuppressLeadingZero>Y</SuppressLeadingZero>
</InstantaneousDemand>
</rainforest>`

func TestParse_InstantaneousDemand(t *testing.T) {
	rf, err := Parse([]byte(instantaneousDemandFixture))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rf.InstantaneousDemand == nil {
		t.Fatal("expected InstantaneousDemand fragment, got nil")
	}

	kw, err := rf.InstantaneousDemand.KW()
	if err != nil {
		t.Fatalf("KW() error = %v", err)
	}
	if want := 5.944; kw != want {
		t.Errorf("KW() = %v, want %v", kw, want)
	}
}

func TestInstantaneousDemand_NegativeDemand(t *testing.T) {
	// 0xFFFFFF is -1 as a 24-bit signed integer (two's complement).
	d := &InstantaneousDemand{Demand: "0xFFFFFF", Multiplier: "0x00000001", Divisor: "0x00000001"}
	kw, err := d.KW()
	if err != nil {
		t.Fatalf("KW() error = %v", err)
	}
	if want := -1.0; kw != want {
		t.Errorf("KW() = %v, want %v", kw, want)
	}
}

func TestInstantaneousDemand_ZeroMultiplierDivisorTreatedAsOne(t *testing.T) {
	d := &InstantaneousDemand{Demand: "0x00000A", Multiplier: "0x00000000", Divisor: "0x00000000"}
	kw, err := d.KW()
	if err != nil {
		t.Fatalf("KW() error = %v", err)
	}
	if want := 10.0; kw != want {
		t.Errorf("KW() = %v, want %v", kw, want)
	}
}

const currentSummationFixture = `<?xml version="1.0"?>
<rainforest macId="0xf0ad4e00ce69" timestamp="1355292588s">
<CurrentSummationDelivered>
<DeviceMacId>0x00158d0000000004</DeviceMacId>
<MeterMacId>0x00178d0000000004</MeterMacId>
<TimeStamp>0x185adc1d</TimeStamp>
<SummationDelivered>0x00000000015F90</SummationDelivered>
<SummationReceived>0x0000000000000A</SummationReceived>
<Multiplier>0x00000001</Multiplier>
<Divisor>0x000003e8</Divisor>
<DigitsRight>0x03</DigitsRight>
<DigitsLeft>0x00</DigitsLeft>
<SuppressLeadingZero>Y</SuppressLeadingZero>
</CurrentSummationDelivered>
</rainforest>`

func TestParse_CurrentSummationDelivered(t *testing.T) {
	rf, err := Parse([]byte(currentSummationFixture))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rf.CurrentSummationDelivered == nil {
		t.Fatal("expected CurrentSummationDelivered fragment, got nil")
	}

	delivered, err := rf.CurrentSummationDelivered.DeliveredKWh()
	if err != nil {
		t.Fatalf("DeliveredKWh() error = %v", err)
	}
	if want := 90.0; delivered != want { // 0x15F90 = 90000; 90000/1000 = 90
		t.Errorf("DeliveredKWh() = %v, want %v", delivered, want)
	}

	received, err := rf.CurrentSummationDelivered.ReceivedKWh()
	if err != nil {
		t.Fatalf("ReceivedKWh() error = %v", err)
	}
	if want := 0.01; received != want { // 0xA = 10; 10/1000 = 0.01
		t.Errorf("ReceivedKWh() = %v, want %v", received, want)
	}
}

func TestParse_InvalidXML(t *testing.T) {
	if _, err := Parse([]byte("not xml")); err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
}

// Fixture captured live from a real Rainforest RFA-Z109 EAGLE upload.
const priceClusterFixture = `<?xml version="1.0"?>
<rainforest macId="0xd8d5b90019dc" version="undefined" timestamp="1789531271s">
<PriceCluster>
  <DeviceMacId>0xd8d5b900000032a1</DeviceMacId>
  <MeterMacId>0x0007810000b3b716</MeterMacId>
  <TimeStamp>0xffffffff</TimeStamp>
  <Price>0x00000449</Price>
  <Currency>0x007c</Currency>
  <TrailingDigits>0x04</TrailingDigits>
  <Tier>0x01</Tier>
  <StartTime>0xffffffff</StartTime>
  <Duration>0xffff</Duration>
  <RateLabel>Block 1</RateLabel>
  <Port>/dev/ttySP0</Port>
</PriceCluster>

</rainforest>`

func TestParse_PriceCluster(t *testing.T) {
	rf, err := Parse([]byte(priceClusterFixture))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rf.PriceCluster == nil {
		t.Fatal("expected PriceCluster fragment, got nil")
	}

	price, err := rf.PriceCluster.PricePerUnit()
	if err != nil {
		t.Fatalf("PricePerUnit() error = %v", err)
	}
	if want := 0.1097; price != want {
		t.Errorf("PricePerUnit() = %v, want %v", price, want)
	}

	if got, want := rf.PriceCluster.CurrencyName(), "CAD"; got != want {
		t.Errorf("CurrencyName() = %q, want %q", got, want)
	}

	if got, want := rf.PriceCluster.RateLabel, "Block 1"; got != want {
		t.Errorf("RateLabel = %q, want %q", got, want)
	}
}

func TestPriceCluster_CurrencyName_UnknownCodeFallsBackToNumeric(t *testing.T) {
	p := &PriceCluster{Currency: "0x0001"}
	if got, want := p.CurrencyName(), "1"; got != want {
		t.Errorf("CurrencyName() = %q, want %q", got, want)
	}
}

func TestPriceCluster_PricePerUnit_ZeroTrailingDigits(t *testing.T) {
	p := &PriceCluster{Price: "0x0000000A", TrailingDigits: "0x00"}
	price, err := p.PricePerUnit()
	if err != nil {
		t.Fatalf("PricePerUnit() error = %v", err)
	}
	if want := 10.0; price != want {
		t.Errorf("PricePerUnit() = %v, want %v", price, want)
	}
}
