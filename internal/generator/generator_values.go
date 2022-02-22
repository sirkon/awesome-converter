package generator

import (
	"fmt"
	"go/types"
	"strings"

	"gitlab.stageoffice.ru/UCS-COMMON/matiss/v2"
)

// convertValue конвертация данного значения заданного переменной src в приёмник dst
// TODO придумать способ показывать имена ключей в ошибках. Кажется, проще всего добавлять их в контекст
//      ошибки.
func (g *Generator) convertValue(
	r *matiss.GoRenderer,
	dst string,
	dstType types.Type,
	src string,
	srcType types.Type,
	descr FieldMatchDescription,
	whoami string,
	noNilGuard bool,
) {
	if _, ok := descr.(*FieldMatchNoMatch); ok {
		return
	}

	nilGuarded := is[*types.Pointer](srcType) || is[*types.Slice](srcType) || is[*types.Map](srcType)
	nilGuarded = nilGuarded && !noNilGuard

	if nilGuarded {
		r.L(`if $0 != nil {`, src)
	}

	switch v := descr.(type) {
	case *FieldMatchNoMatch:
		return

	case *FieldMatchDirect:
		assign(r, dst, dstType, src, srcType)

	case *FieldMatchConversion:
		var call string
		var sig *types.Signature
		switch {
		case v.MethodPrimary != "":
			sig = lookForMethod(unpointer(srcType).(*types.Named), v.MethodPrimary).Type().(*types.Signature)
			call = r.S("$0.$1()", src, v.MethodPrimary)
		case v.PrimaryToSecondary != "":
			fn := lookForFunc(unpointer(srcType).(*types.Named), v.PrimaryToSecondary)
			sig = fn.Type().(*types.Signature)
			arg := rightReference(src, srcType, sig.Params().At(0).Type())
			call = r.S("$0($1)", g.callName(r, fn), arg)
		case v.SecondaryFromPrimary != "":
			fn := lookForFunc(unpointer(dstType).(*types.Named), v.SecondaryFromPrimary)
			sig = fn.Type().(*types.Signature)
			arg := rightReference(src, srcType, sig.Params().At(0).Type())
			call = r.S("$0($1)", g.callName(r, fn), arg)
		}

		switch sig.Results().Len() {
		case 1:
			assignSafe(r, dst, dstType, call, sig.Results().At(0).Type(), nilGuarded)
		case 2:
			r.Imports().Errors().Ref("errors")
			if nilGuarded {
				r.L(`convres, err := $0`, call)
				r.L(`if err != nil {`)
				r.L(
					`    return nil, $errors.Wrap(err, "convert $0").Any("invalid-$1", $2)`,
					whoami,
					humanGuess(src),
					src,
				)
				r.L(`}`)
				r.N()
				assign(r, dst, dstType, "convres", sig.Results().At(0).Type())
			} else {
				// вначале проверка err == nil потому что err != nil менее вероятная ситуация в данном случае
				r.L(`if convres, err := $0; err == nil {`, call)
				assign(r, dst, dstType, "convres", sig.Results().At(0).Type())
				r.L(`} else {`)
				r.L(
					`    return nil, $errors.Wrap(err, "convert $0").Any("invalid-$1", $2)`,
					whoami,
					humanGuess(src),
					src,
				)
				r.L(`}`)
			}
		}

	case *FieldMatchEnum:
		r.Imports().Errors().Ref("errors")

		if v.Secondary.isProto {
			r.L(`if enumval, ok := $0_value[int32($1)]; ok {`, g.typeName(r, dstType), deref(src, srcType))
			assign(r, dst, dstType, "enumval", srcType)
			r.L(`} else {`)
			r.L(`    return $errors.Newf("unknown value %v of $0", $1)`, src, deref(src, srcType))
			r.L(`}`)
		} else {
			r.L(`switch $0 {`, deref(src, srcType))
			for _, v := range v.Secondary.values {
				r.L(`case $0:`, v.Val().ExactString())
				assignSafe(
					r,
					dst,
					dstType,
					g.typeName(r, v.Type())+"("+v.Val().ExactString()+")",
					v.Type(),
					true,
				)
			}
			r.L(`default:`)
			r.L(`    return nil, $errors.Newf("unknown value %v of $0", $1)`, whoami, deref(src, srcType))
			r.L(`}`)
		}

	case *FieldMatchCastable:
		assignSafe(
			r,
			dst,
			dstType,
			r.S("$0($1)", g.typeName(r, unpointer(dstType)), deref(src, srcType)),
			unpointer(srcType),
			nilGuarded,
		)

	case *FieldMatchSlice:
		if !nilGuarded && isPointer(dstType) {
			r.L(`{`)
		}

		var tmpDst string
		if isPointer(dstType) {
			tmpDst = "tmpslice"
			r.L(`var $0 $1`, tmpDst, g.typeName(r, unpointer(dstType)))
		} else {
			tmpDst = dst
		}

		r.L(`$0 = make($1, len($2))`, tmpDst, g.typeName(r, unpointer(dstType)), deref(src, srcType))
		r.L(`for i, elemval := range $0 {`, deref(src, srcType))
		g.convertValue(
			r,
			tmpDst+"[i]",
			unpointer(dstType).(*types.Slice).Elem(),
			"elemval",
			unpointer(srcType).(*types.Slice).Elem(),
			v.Elem,
			"slice element of "+whoami,
			false,
		)

		if isPointer(dstType) {
			r.L(`$0 = &$1`, dst, tmpDst)
		}
		r.L(`}`)

		if !nilGuarded && isPointer(dstType) {
			r.L(`}`)
		}

	case *FieldMatchMap:
		if !nilGuarded && isPointer(dstType) {
			r.L(`{`)
		}

		var tmpDst string
		if isPointer(dstType) {
			tmpDst = "tmpmap"
			r.L(`var $0 $1`, tmpDst, g.typeName(r, unpointer(dstType)))
		} else {
			tmpDst = dst
		}

		r.L(`$0 = make($1, len($2))`, tmpDst, g.typeName(r, unpointer(dstType)), deref(src, srcType))
		r.L(`for keyval, elemval := range $0 {`, deref(src, srcType))
		g.convertValue(r, tmpDst+"[keyval]", unpointer(dstType).(*types.Map).Elem(), "elemval", unpointer(srcType).(*types.Map).Elem(), v.Elem, "map element of "+whoami, false)

		if isPointer(dstType) {
			r.L(`$0 = &$1`, dst, tmpDst)
		}
		r.L(`}`)

		if !nilGuarded && isPointer(dstType) {
			r.L(`}`)
		}
	}

	if nilGuarded {
		r.L(`}`)
	}
}

func (g *Generator) callName(r *matiss.GoRenderer, fn *types.Func) string {
	if g.prim.Obj().Pkg().Path() == fn.Pkg().Path() {
		return fn.Name()
	}

	refname := fmt.Sprintf("callpkg%d", g.fqsec)
	g.fqsec++
	r.Imports().Add(fn.Pkg().Path()).Ref(refname)
	return r.S(`$`+refname+`.$0`, fn.Name())
}

// assign генерация присваивания значения поля от другого значения
func assign(
	r *matiss.GoRenderer,
	dst string,
	dstType types.Type,
	src string,
	srcType types.Type,
) {
	switch {
	case isPointer(srcType) && !isPointer(dstType):
		r.L(`$0 = *$1`, dst, src)
	case !isPointer(srcType) && isPointer(dstType):
		if zero := basicZero(srcType); zero != "" {
			r.L(`if $0 != $1 {`, src, zero)
			r.L(`    $0 = &$1`, dst, src)
			r.L(`}`)
			return
		}

		r.L(`$0 = &$1`, dst, src)
	default:
		r.L(`$0 = $1`, dst, src)
	}
}

// assignSafe генерация присваивания значения поля от другого значения которое не имеет адреса
func assignSafe(
	r *matiss.GoRenderer,
	dst string,
	dstType types.Type,
	src string,
	srcType types.Type,
	guarded bool,
) {
	switch {
	case isPointer(srcType) && !isPointer(dstType):
		r.L(`$0 = *$1`, dst, src)
	case !isPointer(srcType) && isPointer(dstType):
		if zero := basicZero(srcType); zero != "" {
			r.L(`if tmp := $0; tmp != $1 {`, src, zero)
			r.L(`    $0 = &tmp`, dst)
			r.L(`}`)
			return
		}

		if !guarded {
			r.L(`{`)
		}
		r.L(`tmp := $0`, src)
		r.L(`$0 = &tmp`, dst)
		if !guarded {
			r.L(`}`)
		}
	default:
		r.L(`$0 = $1`, dst, src)
	}
}

func lookForMethod(x *types.Named, name string) *types.Func {
	for i := 0; i < x.NumMethods(); i++ {
		if m := x.Method(i).Name(); m == name {
			return x.Method(i)
		}
	}

	return nil
}

func lookForFunc(x *types.Named, name string) *types.Func {
	scope := x.Obj().Pkg().Scope()
	return scope.Lookup(name).(*types.Func)
}

func rightReference(src string, srcType, dstType types.Type) string {
	switch {
	case isPointer(srcType) && !isPointer(dstType):
		return "*" + src
	case !isPointer(srcType) && isPointer(dstType):
		return "&" + src
	default:
		return src
	}
}

func deref(src string, srcType types.Type) string {
	if isPointer(srcType) {
		return "*" + src
	}

	return src
}

func unpointer(t types.Type) types.Type {
	if v, ok := t.(*types.Pointer); ok {
		return v.Elem()
	}

	return t
}

func basicZero(t types.Type) string {
	switch v := t.(type) {
	case *types.Pointer:
		return ""
	case *types.Named:
		return basicZero(v.Underlying())
	case *types.Basic:
		switch v.Kind() {
		case types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64:
			return "0"
		case types.String:
			return `""`
		case types.Bool:
			return "false"
		default:
			return ""
		}
	default:
		return ""
	}
}

// humanGuess костыль для получения названия поля из названия src
func humanGuess(src string) string {
	cut, after, found := strings.Cut(src, ".")
	if !found {
		after = cut
	}

	return matiss.Striked(after)
}
