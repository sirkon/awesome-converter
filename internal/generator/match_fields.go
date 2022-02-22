package generator

import (
	"go/types"

	"github.com/sirkon/message"

	"gitlab.stageoffice.ru/UCS-COMMON/errors"
	"gitlab.stageoffice.ru/UCS-COMMON/matiss/v2"
)

// fieldMatchInfo тип сопоставляющий поля из первичной и вторичной структур
type fieldMatchInfo struct {
	prim  *types.Var
	sec   *types.Var
	descr FieldMatchDescription
}

// fieldSecondaryOneof тип сопоставляющий полю oneof-а из secondary-типа поля из primary-типа
type fieldSecondaryOneof struct {
	sec      *types.Var
	branches []fieldBranchDescr
}

// fieldBranchDescr описание ветви и указание поля из primary-типа
type fieldBranchDescr struct {
	// название ветви
	branch string
	// геттер возвращающий обёртку для ветви
	getter *types.Func
	// поле в primary-типе соответствующее ветви
	prim *types.Var
	// "настоящая" ветвь (поле в обёртке ветви)
	sec *types.Var
	// descr описание конвертации между полем в primary и содержимым ветви в secondary структурах
	descr FieldMatchDescription
}

// getFieldsMatches поиск эквивалентных полей.
// Критерий эквивалентности полей, должны выполняться оба условия:
//     • Совпадают значения полученные из имён полей с помощью matiss.Underscored либо вручную задано сопоставление
//        одного поля другому в словаре manual (сейчас не заполняется)
//     • Сопоставленные по имени поля имеют эквивалентные типы.
// Критерий эквивалентности типа:
//   Типы полей U и V являются эквивалентными (U ~ V) если выполняется одно из следующих условий (в порядке уменьшения
//   приоритета):
//     • X ~ X
//     • X ~ *X
//     • В пакете с типом U или V доступны ПУБЛИЧНЫЕ функции и/или методы преобразования из типа X в тип Y и обратно,
//       где X ~ U и Y ~ V. Данные функции методы не должны принимать никаких аргументов и возвращать либо результат
//       типа (*)V, или ((*)V, error).
//     • Типы U и V:
//         • Являются перечислениями в смысле Go (определяются функцией getEnumInfo)
//         • Значения перечислений совпадают либо, в случае перечислений-строк, значения типа U являются суффиксами
//           значений V или наоборот. Например, пара DBRegionID и RegionId удовлетворяют этому значению.
//         • TODO сформулировать критерий совпадения по именам значений — это чуть сложнее, должны до какой-то степени
//                совпадать суффиксы.
//         • Warning: если типы оба являются перечислениями но не выполняются критерии из этого подпункта, то
//                    они НЕ являются эквивалентными.
//     • X и Y приводятся друг к другу и X ~ U, Y ~ V
//     • []X ~ []Y если X ~ Y
//     • map[A]B ~ map[X]Y если A ~ X и B ~ Y
//   Warning: целочисленные типы различных размерностей, например int8 и uin64, считаются эквивалентными в рамках
//            данных критериев.
//
// Кроме этого, заводится специальный костыль для полей соответсвующих oneof для структур сгенерированных
// protoc-gen-go. Такие поля ищутся только в secondary-типе следующим образом:
//   1. Ищем поля с типом–интерфейсом is<type_name>_<field name>
//   2. Проверяем, что найденный интерфейс содержит одноимённый метод
//   3. Ищем реализации интерфейса (по методу)
//   4. Для каждой из реализаций ищем поле с эквивалентным именем (описано выше) в primary-типе, причём такое поле
//      не должно быть сопоставленным другому.
//   5. Если тип только найденного поля эквивалентен типу поля в ветви, то считается что найдено соответствие между
//      ветвью и полем в primary-типе
//   6. Если для всех ветвей было найдено соответствие в полях, то такие поля удаляются из поматченных
//
// TODO реализовать наполнение словаря manual для ручного указания эквивалентных полей
func (g *Generator) getFieldsMatches(manual map[string]string) ([]fieldMatchInfo, []fieldSecondaryOneof) {
	prim := g.prim.Underlying().(*types.Struct)
	sec := g.sec.Underlying().(*types.Struct)

	var errorsHappened bool
	var res []fieldMatchInfo
outer:
	for i := 0; i < prim.NumFields(); i++ {
		pf := prim.Field(i)
		if pf.Name() == "" || !pf.Exported() {
			continue
		}

		if err := checkTypeSupport(pf.Type()); err != nil {
			message.Errorf("%s %s", g.fs.Position(pf.Pos()), err)
			errorsHappened = true
		}

		want := matiss.Underscored(pf.Name())
		if name, ok := manual[want]; ok {
			want = name
		}

		for j := 0; j < sec.NumFields(); j++ {
			ps := sec.Field(j)
			if matiss.Underscored(ps.Name()) != want {
				continue
			}

			eq := g.getTypeMatchDescription(pf.Type(), ps.Type())
			res = append(res, fieldMatchInfo{
				prim:  pf,
				sec:   ps,
				descr: eq,
			})
			continue outer
		}

		res = append(res, fieldMatchInfo{
			prim:  pf,
			descr: &FieldMatchNoMatch{},
		})
	}

	if errorsHappened {
		message.Fatal("unhandled types met, cannot continue")
	}

	res, oneofs := g.matchOneofs(sec, res)

	return res, oneofs
}

func (g *Generator) matchOneofs(sec *types.Struct, res []fieldMatchInfo) ([]fieldMatchInfo, []fieldSecondaryOneof) {
	// ищем oneof-поля
	var oneofs []fieldSecondaryOneof
oouter:
	for i := 0; i < sec.NumFields(); i++ {
		field := sec.Field(i)

		t, ok := field.Type().(*types.Named)
		if !ok {
			continue
		}

		magicOneofName := "is" + g.sec.Obj().Name() + "_" + field.Name()
		if t.Obj().Name() != magicOneofName {
			// тип поля соотв. oneof должно называться is<type name>_<oneof field name>
			continue
		}

		tt, ok := t.Underlying().(*types.Interface)

		// и этот тип должен иметь единственный метод имеющий такое же название
		if tt.NumMethods() != 1 {
			continue
		}

		m := tt.Method(0)
		if m.Name() != magicOneofName {
			continue
		}

		// ищем в пакете с secondary-типом все типы реализующие данный инторфейс
		branches := g.getOneofImpls(m.Pkg().Scope(), magicOneofName)
		if len(branches) == 0 {
			continue
		}

		// Ветви найдены, находим содержимое каждой из них и ищем геттер на secondary-структуре, который возвращает
		// значение данного типа.
		// Далее, в типе-ветви должно быть поле, название которого должно совпадать с полем в primary структуре
		// которое не сопоставлено никакому другому, а его тип должен быть эквивалентен типу соотв. primary–поля.

		var oneof []fieldBranchDescr
		exclude := map[int]struct{}{}
		for _, branch := range branches {
			ns, ok := branch.Obj().Type().(*types.Named)
			if !ok {
				continue
			}
			s, ok := ns.Underlying().(*types.Struct)
			if !ok {
				continue
			}

			f := s.Field(0)

			for j := 0; j < g.sec.NumMethods(); j++ {
				mt := g.sec.Method(j)
				if mt.Name() != matiss.Proto("get", f.Name()) {
					continue
				}

				resType := mt.Type().(*types.Signature).Results().At(0).Type()
				match := g.getTypeMatchDescription(resType, f.Type())
				if _, ok := match.(*FieldMatchNoMatch); ok {
					continue oouter
				}

				// геттер для ветви найден, проверяем его тип и что соотв. поле не входит в число заматченных
				// на предыдущем этапе
				for i, m := range res {
					if _, ok := m.descr.(*FieldMatchNoMatch); !ok {
						continue
					}

					if matiss.Underscored(m.prim.Name()) != matiss.Underscored(f.Name()) {
						continue
					}

					match := g.getTypeMatchDescription(m.prim.Type(), resType)
					if _, ok := match.(*FieldMatchNoMatch); ok {
						continue
					}

					// поле с именем бранча имеет эквивалентный тип и не сопоставлено никакому другому, добавляем
					// ветвь
					oneof = append(oneof, fieldBranchDescr{
						branch: m.prim.Name(),
						getter: mt,
						prim:   m.prim,
						sec:    s.Field(0),
						descr:  match,
					})
					exclude[i] = struct{}{}
				}

			}
		}

		if len(oneof) == len(branches) {
			oneofs = append(oneofs, fieldSecondaryOneof{
				sec:      field,
				branches: oneof,
			})

			// надо убрать поматченные поля
			var newres []fieldMatchInfo
			for i, item := range res {
				if _, ok := exclude[i]; ok {
					continue
				}

				newres = append(newres, item)
			}
			res = newres
		}
	}
	return res, oneofs
}

func (g *Generator) getTypeMatchDescription(prim, sec types.Type) FieldMatchDescription {
	// если один и тот же тип
	if prim == sec || types.AssignableTo(prim, sec) {
		return &FieldMatchDirect{}
	}

	// убираем указатели

	// вначале на prim
	if v, ok := prim.(*types.Pointer); ok {
		return g.getTypeMatchDescription(v.Elem(), sec)
	}

	// потом на sec
	if v, ok := sec.(*types.Pointer); ok {
		return g.getTypeMatchDescription(prim, v.Elem())
	}

	// если имеются функции преобразования между типами
	if v, ok := g.thereIsConversion(prim, sec); ok {
		return v
	}

	// функции преобразования могут быть из secondary в primary
	if v, ok := g.thereIsConversion(sec, prim); ok {
		// меняем местами направление методов
		return &FieldMatchConversion{
			MethodPrimary:        "",
			PrimaryToSecondary:   "",
			PrimaryFromSecondary: "",
			MethodSecondary:      v.MethodPrimary,
			SecondaryToPrimary:   v.PrimaryToSecondary,
			SecondaryFromPrimary: v.PrimaryFromSecondary,
		}
	}

	// если подозрительно похожие енумии
	penum, senum, enummatch := g.matchEnums(prim, sec)
	switch enummatch {
	case enumMatchStateNotEnums:
		// оба не енумии, продолжаем проверку дальше
	case enumMatchStateOneIsNotEnum, enumMatchStateDifferentEnums:
		return &FieldMatchNoMatch{}
	case enumMatchStateMatched:
		return &FieldMatchEnum{
			Primary:   penum,
			Secondary: senum,
		}
	}

	// типы могут приводиться друг к другу
	if types.AssignableTo(prim, sec) {
		return &FieldMatchCastable{}
	}

	// named-тип может быть построен на кастуемом к sec типе, поэтому снимаем имена и доходим до нижнего
	// но наверное здесь нужно аккуратнее и не снимать все *Nameds сразу а дёргать по-одному
	pun := stripNameds(prim)
	sun := stripNameds(sec)
	if pun != nil && sun != nil {
		if types.AssignableTo(pun, sun) {
			return &FieldMatchCastable{}
		}
	}

	// в случае слайсов типы должны быть эквивалентными
	sliceMatchDescr, sliceMatch := g.areEquivalentSlices(prim, sec)
	switch sliceMatch {
	case sliceMatchStateNoSlices:
		// оба не слайсы, продолжаем дальше
	case sliceMatchStateIncompatibleWithSlice, sliceMatchStateDifferentSlices:
		return &FieldMatchNoMatch{}
	case sliceMatchStateMatched:
		return sliceMatchDescr
	}

	// в случае словарей типы и ключей, и значений так же должны быть эквивалентыми
	mapMatchDecr, mapMatch := g.areEquivalentMaps(prim, sec)
	switch mapMatch {
	case mapMatchStateNomaps:
		// оба не мапы, продолжаем дальше
	case mapMatchStateIncompatibleWithMap, mapMatchStateDifferentMaps:
		return &FieldMatchNoMatch{}
	case mapMatchStateMatched:
		return mapMatchDecr
	}

	if basicAssignable(prim, sec) {
		return &FieldMatchCastable{}
	}

	return &FieldMatchNoMatch{}
}

func stripNameds(n types.Type) types.Type {
	v, ok := n.(*types.Named)
	if !ok {
		return n
	}

	if v.Underlying() == v {
		return nil
	}

	return stripNameds(v.Underlying())
}

func basicAssignable(x, y types.Type) bool {
	var xb *types.Basic
	switch v := x.(type) {
	case *types.Basic:
		xb = v
	case *types.Named:
		if v.Underlying() != v {
			return basicAssignable(v.Underlying(), y)
		}
	default:
		return false
	}

	var yb *types.Basic
	switch v := y.(type) {
	case *types.Basic:
		yb = v
	case *types.Named:
		if v.Underlying() != v {
			return basicAssignable(x, v.Underlying())
		}
	default:
		return false
	}

	if types.AssignableTo(xb, yb) {
		return true
	}

	switch xb.Kind() {
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
		types.Float32, types.Float64:
		switch yb.Kind() {
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func canBeEnum(t types.Type) bool {
	switch v := t.(type) {
	case *types.Basic:
		switch v.Kind() {
		case
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Byte, types.Uint16, types.Uint32, types.Uint64,
			types.String:
			return true
		}
	case *types.Named:
		return canBeEnum(v.Obj().Type())
	case *types.Pointer:
		return canBeEnum(v.Elem())
	default:
		if t.Underlying() != nil && t.Underlying() != t {
			return canBeEnum(t.Underlying())
		}
	}

	return false
}

func (g *Generator) reportMatchingInfo(
	m []fieldMatchInfo,
	oos []fieldSecondaryOneof,
) (missingPrimary bool, missingSecondary bool) {
	message.Info("\nregular fields matches")

	for _, info := range m {
		if _, ok := info.descr.(*FieldMatchNoMatch); ok {
			missingPrimary = true
		}

		if info.sec != nil {
			message.Infof(
				"primary %s(%s) ↔ secondary %s(%s): %s",
				info.prim.Name(),
				info.prim.Type(),
				info.sec.Name(),
				info.sec.Type(),
				info.descr,
			)
		} else {
			message.Warningf("primary field %s (%s): %s", info.prim.Name(), info.prim.Type(), info.descr)
		}
	}

	for i, oo := range oos {
		if i == 0 {
			message.Info("\noneof matches")
		}
		for _, branch := range oo.branches {
			message.Infof(
				"primary field %s (%s) ↔ secondary oneof branch %s (%s) of %s: %s",
				branch.prim.Name(),
				branch.prim.Type(),
				branch.branch,
				branch.sec.Type(),
				oo.sec.Name(),
				branch.descr,
			)
		}
	}

	missingSecondary = g.secondaryHasUncoveredFields(m, oos)

	message.Info()

	if missingPrimary {
		message.Warning("not all primary fields were matched")
	}

	if missingSecondary {
		message.Warning("not all secondary fields were matched")
	}

	return missingPrimary, missingSecondary
}

// secondaryHasUncoveredFields выяснение, что имеются публичные поля в secondary-типе для которых не найдено
// соответствие в primary.
func (g *Generator) secondaryHasUncoveredFields(ms []fieldMatchInfo, oos []fieldSecondaryOneof) bool {
	sec := g.sec.Underlying().(*types.Struct)

outer:
	for i := 0; i < sec.NumFields(); i++ {
		f := sec.Field(i)
		if !f.Exported() {
			continue
		}

		for _, m := range ms {
			if m.sec == nil {
				continue outer
			}

			if m.sec.Id() == f.Id() {
				continue outer
			}
		}

		for _, oo := range oos {
			if oo.sec.Id() == f.Id() {
				continue outer
			}
		}

		return true
	}

	return false
}

// checkTypeSupport не все типы разрешены
func checkTypeSupport(t types.Type) error {
	switch v := t.(type) {
	case *types.Pointer:
		var root string
		switch v.Elem().(type) {
		case *types.Pointer:
			root = "pointers of pointers"
		case *types.Slice:
			root = "pointers of slices"
		case *types.Map:
			root = "pointers of maps"
		}

		if root != "" {
			return errors.Newf("conversions of %s are not supported", root)
		}
	case *types.Chan:
		return errors.New("conversions of channels makes no sense")
	case *types.Signature:
		return errors.Newf("conversions of functions are impossible")
	}

	return nil
}
