package generator

import "go/types"

// thereIsConversion поиск функций преобразования из типа prim или *prim в тип sec или *sec.
// Это должен быть именованный тип. По предыдущим шагам в getTypeMatchDescription оба полученных на данном
// этапе типов не являются указателями.
func (g *Generator) thereIsConversion(prim, sec types.Type) (*FieldMatchConversion, bool) {
	// типы должны быть именованными, чтобы иметь методы
	x, ok := prim.(*types.Named)
	if !ok {
		return nil, false
	}

	// вначале ищем метод prim -> sec или *sec.
	var conv FieldMatchConversion
	for i := 0; i < x.NumMethods(); i++ {
		m := x.Method(i)
		if !m.Exported() {
			continue
		}

		// у метода не должно быть параметров
		sig := m.Type().(*types.Signature)
		if sig.Params().Len() != 0 {
			continue
		}

		// возвращаться может только sec или *sec, может быть с error-ом
		res := sig.Results()
		if g.isProperConversionResult(res, sec) {
			conv.MethodPrimary = m.Name()
			break
		}
	}

	primscope := x.Obj().Pkg().Scope()

	if conv.MethodPrimary == "" {
		// метод не найден, ищем в пакете "свободную" функцию (*)prim -> (*)sec, сигнатура которой
		// должна иметь вид func ((*)prim) (*)sec или func ((*)prim) ((*)sec, error)
		v, ok := g.hasPrimToSecMethod(prim, sec, primscope)
		if !ok {
			return nil, false
		}

		conv.PrimaryToSecondary = v
	}

	// должна быть и функция преобразующая sec в prim
	if v, ok := g.hasPrimToSecMethod(sec, prim, primscope); ok {
		conv.PrimaryFromSecondary = v
		return &conv, true
	}

	return nil, false
}

func (g *Generator) hasPrimToSecMethod(prim types.Type, sec types.Type, primscope *types.Scope) (string, bool) {
	for _, name := range primscope.Names() {
		f, ok := primscope.Lookup(name).(*types.Func)
		if !ok {
			continue
		}

		if !f.Exported() {
			continue
		}

		sig := f.Type().(*types.Signature)
		if sig.Recv() != nil {
			// методы уже просматривали
			continue
		}

		if !g.isProperConversionResult(sig.Results(), sec) {
			continue
		}

		if sig.Params().Len() != 1 {
			continue
		}

		paramType := sig.Params().At(0).Type()
		if v, ok := paramType.(*types.Pointer); ok {
			if types.AssignableTo(v.Elem(), prim) {
				return name, true
			}
		} else {
			if types.AssignableTo(paramType, prim) {
				return name, true
			}
		}
	}

	return "", false
}

func (g *Generator) isProperConversionResult(res *types.Tuple, sec types.Type) bool {
	switch res.Len() {
	case 2:
		// если возвращаются два значения то вторым может и должен быть error
		if res.At(1).Type().String() != "error" {
			return false
		}
		fallthrough

	case 1:
		// первое (может быть единственное) возвращаемое значение должно иметь тип sec или *sec
		if v, ok := res.At(0).Type().(*types.Pointer); ok {
			if v.Elem() == sec {
				return true
			}
		} else {
			vv := res.At(0).Type()
			if types.AssignableTo(vv, sec) {
				return true
			}
		}

	default:
		return false
	}
	return false
}
