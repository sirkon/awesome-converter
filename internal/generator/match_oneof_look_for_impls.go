package generator

import "go/types"

func (g *Generator) getOneofImpls(scope *types.Scope, methodName string) []*types.Named {
	var res []*types.Named
outer:
	for _, n := range scope.Names() {
		item := scope.Lookup(n)
		if !item.Exported() {
			continue
		}

		t, ok := item.Type().(*types.Named)
		if !ok {
			continue
		}

		// тип должен быть структурой
		if _, ok := t.Underlying().(*types.Struct); !ok {
			continue
		}

		// ищем магический метод
		for i := 0; i < t.NumMethods(); i++ {
			if t.Method(i).Name() != methodName {
				continue outer
			}
		}

		// значит, эта структура относится к одной из ветвей
		res = append(res, t)
	}

	return res
}
