package service

import (
	"math"
	"testing"

	"qjrxy/internal/cpdaily"
)

// TestJitterCoordWithinTenMeters 校验随机偏移不超过 10 米。
func TestJitterCoordWithinTenMeters(t *testing.T) {
	baseLon, baseLat := 116.397428, 39.909230
	for i := 0; i < 500; i++ {
		lon, lat := jitterCoord(baseLon, baseLat)
		d := cpdaily.Haversine(baseLon, baseLat, lon, lat)
		if d > 10.01 {
			t.Fatalf("偏移 %.2f 米，超过 10 米上限（lon=%.8f lat=%.8f）", d, lon, lat)
		}
		if math.Abs(lon-baseLon) < 1e-12 && math.Abs(lat-baseLat) < 1e-12 {
			t.Fatal("偏移不应恒为 0")
		}
	}
}

// TestJitterCoordZeroStaysZero 校验未设置坐标（0,0）时不偏移。
func TestJitterCoordZeroStaysZero(t *testing.T) {
	lon, lat := jitterCoord(0, 0)
	if lon != 0 || lat != 0 {
		t.Fatalf("0,0 不应偏移，得到 %.8f,%.8f", lon, lat)
	}
}
