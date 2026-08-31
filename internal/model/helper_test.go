package model

import "strconv"

func strconvParseFloat(s string) (float64, error) { return strconv.ParseFloat(s, 64) }
