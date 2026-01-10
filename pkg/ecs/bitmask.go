package ecs

import "math/bits"

type Bitmask []uint64

func (b Bitmask) Set(bit ComponentID) Bitmask {
	word, pos := bit/64, bit%64
	for len(b) <= int(word) {
		b = append(b, 0)
	}
	b[word] |= (1 << pos)
	return b
}

func (b Bitmask) Matches(required Bitmask) bool {
	if len(b) < len(required) {
		return false
	}
	for i := range required {
		if (b[i] & required[i]) != required[i] {
			return false
		}
	}
	return true
}

func (b Bitmask) ForEachSet(fn func(id ComponentID)) {
	for wordIdx, word := range b {
		if word == 0 {
			continue
		}

		// Dopóki w słowie są jakieś jedynki
		for word != 0 {
			// bits.TrailingZeros64 zwraca liczbę zer przed pierwszą jedynką (od prawej)
			// Jest to bezpośrednio pozycja bitu, który nas interesuje.
			bitPos := bits.TrailingZeros64(word)

			// Obliczamy ID
			id := ComponentID(wordIdx*64 + bitPos)
			fn(id)

			// Czyścimy obsłużony bit, aby przejść do kolejnego
			word &= ^(1 << bitPos)
		}
	}
}

func (b Bitmask) Clear(bit ComponentID) Bitmask {
	word, pos := bit/64, bit%64
	if len(b) <= int(word) {
		return b // Bit i tak nie jest ustawiony, skoro maska jest za krótka
	}
	// Operacja &= ^ (AND NOT) zeruje konkretny bit
	b[word] &= ^(1 << pos)
	return b
}
