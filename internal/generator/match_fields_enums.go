package generator

import "go/types"

type enumMatchState int

const (
	// enumMatchStateNotEnums оба типа не являются енумиями
	enumMatchStateNotEnums enumMatchState = iota
	// enumMatchStateOneIsNotEnum один из типов является енумием, а другой нет, такие типы не могут быть эквивалентными
	enumMatchStateOneIsNotEnum
	// enumMatchStateDifferentEnums оба являются перечислениями, но их значения не совпадают, такие типы не могут быть
	// эквивалентными.
	enumMatchStateDifferentEnums
	// enumMatchStateMatched совпадающие енумии
	enumMatchStateMatched
)

func (g *Generator) matchEnums(prim, sec types.Type) (*enumDescription, *enumDescription, enumMatchState) {
	p := g.getEnumInfo(prim)
	firstIsEnum := p != nil

	s := g.getEnumInfo(sec)
	secondIsEnum := s != nil

	if (firstIsEnum || secondIsEnum) && !(firstIsEnum && secondIsEnum) {
		// случай, когда одно является енумием а другое нет автоматически означает
		// что конвертации никакой не возможно
		return nil, nil, enumMatchStateOneIsNotEnum
	}

	if !firstIsEnum && !secondIsEnum {
		// оба не являются енумиями и это означает что проверять можно дальше
		return nil, nil, enumMatchStateNotEnums
	}

	if len(p.values) != len(s.values) {
		return nil, nil, enumMatchStateDifferentEnums
	}

outer:
	// каждое значение из p.values должно быть в s.values
	for _, pv := range p.values {
		for _, qv := range s.values {
			if pv.Val().String() == qv.Val().String() {
				continue outer
			}
		}

		// не нашли значения pv, выходим
		return nil, nil, enumMatchStateDifferentEnums
	}

	return p, s, enumMatchStateMatched
}
