package spectrum

const Bands = 64

func parseFrame(line []byte, dst []float32) bool {
	if len(dst) < Bands {
		return false
	}

	pos := 0
	for band := range Bands {
		if pos >= len(line) {
			return false
		}
		value := 0
		digits := 0
		for pos < len(line) && line[pos] != ';' {
			c := line[pos]
			if c < '0' || c > '9' {
				return false
			}
			value = value*10 + int(c-'0')
			if value > 1000 {
				return false
			}
			digits++
			pos++
		}
		if digits == 0 || pos >= len(line) {
			return false
		}
		dst[band] = float32(value) / 1000
		pos++
	}
	return pos == len(line)
}
