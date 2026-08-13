package hot

// Scalar sparse partial key matching: return the largest index i < num such
// that dense&pks[i] == pks[i]. Index 0 always matches because the first
// sparse partial key is trivially zero, so the loops only scan down to 1.

func match8scalar(pks *[maxFanout]uint8, dense uint8, num uint8) int {
	for i := int(num) - 1; i > 0; i-- {
		if pks[i]&dense == pks[i] {
			return i
		}
	}
	return 0
}

func match16scalar(pks *[maxFanout]uint16, dense uint16, num uint8) int {
	for i := int(num) - 1; i > 0; i-- {
		if pks[i]&dense == pks[i] {
			return i
		}
	}
	return 0
}

func match32scalar(pks *[maxFanout]uint32, dense uint32, num uint8) int {
	for i := int(num) - 1; i > 0; i-- {
		if pks[i]&dense == pks[i] {
			return i
		}
	}
	return 0
}
