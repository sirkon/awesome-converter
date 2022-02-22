package generator

import "go/types"

type sliceMatchState int

const (
	// sliceMatchStateNoSlices ни один из типов не является слайсом
	sliceMatchStateNoSlices sliceMatchState = iota
	// sliceMatchStateIncompatibleWithSlice один из типов не совместим со слайсом
	sliceMatchStateIncompatibleWithSlice
	// sliceMatchStateDifferentSlices слайсы состоят не из эквивалентых элементов
	sliceMatchStateDifferentSlices
	// sliceMatchStateMatched типы являются слайсами с эквивалентными типами
	sliceMatchStateMatched
)

func (g *Generator) areEquivalentSlices(prim, sec types.Type) (*FieldMatchSlice, sliceMatchState) {
	p, pok := prim.(*types.Slice)
	s, sok := sec.(*types.Slice)

	if !pok && !sok {
		return nil, sliceMatchStateNoSlices
	}

	if (pok || sok) && !(pok && sok) {
		return nil, sliceMatchStateIncompatibleWithSlice
	}

	x := g.getTypeMatchDescription(p.Elem(), s.Elem())
	switch v := x.(type) {
	case *FieldMatchNoMatch:
		return nil, sliceMatchStateDifferentSlices
	default:
		return &FieldMatchSlice{
			Elem: v,
		}, sliceMatchStateMatched
	}
}
