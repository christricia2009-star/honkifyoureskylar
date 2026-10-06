package geo

import "math"

func Miles(lat1, lon1, lat2, lon2 float64) float64 {
	return meters(lat1, lon1, lat2, lon2) / 1609.344
}

func Meters(lat1, lon1, lat2, lon2 float64) float64 {
	return meters(lat1, lon1, lat2, lon2)
}

func meters(lat1, lon1, lat2, lon2 float64) float64 {
	const earth = 6371008.8
	p1 := lat1 * math.Pi / 180
	p2 := lat2 * math.Pi / 180
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earth * c
}
