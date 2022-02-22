package generator

import "go/types"

type mapMatchState int

const (
	// mapMatchStateNomaps ни один из типов не является мапой
	mapMatchStateNomaps mapMatchState = iota
	// mapMatchStateIncompatibleWithMap один из типов не совместим с мапой
	mapMatchStateIncompatibleWithMap
	// mapMatchStateDifferentMaps мапы состоят не из эквивалентых ключей или элементов
	mapMatchStateDifferentMaps
	// mapMatchStateMatched типы являются мапами с эквивалентными типами
	mapMatchStateMatched
)

func (g *Generator) areEquivalentMaps(prim, sec types.Type) (*FieldMatchMap, mapMatchState) {
	p, pok := prim.(*types.Map)
	s, sok := sec.(*types.Map)

	if !pok && !sok {
		return nil, mapMatchStateNomaps
	}

	if (pok || sok) && !(pok && sok) {
		return nil, mapMatchStateIncompatibleWithMap
	}

	var res FieldMatchMap

	kmatch := g.getTypeMatchDescription(p.Key(), s.Key())
	switch v := kmatch.(type) {
	case *FieldMatchNoMatch:
		return nil, mapMatchStateDifferentMaps
	default:
		res.Key = v
	}

	ematch := g.getTypeMatchDescription(p.Elem(), s.Elem())
	switch v := ematch.(type) {
	case *FieldMatchNoMatch:
		return nil, mapMatchStateDifferentMaps
	default:
		res.Elem = v
	}

	return &res, mapMatchStateMatched
}
