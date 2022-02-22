package generator

import (
	"fmt"
	"strings"
)

// FieldMatchDescription is an interface to limit available implementations to partially replicate discriminated union type functionality
type FieldMatchDescription interface {
	fmt.Stringer
	isFieldMatchDescription()
}

// FieldMatchNoMatch branch of FieldMatchDescription
type FieldMatchNoMatch struct{}

func (m *FieldMatchNoMatch) String() string {
	return "no match"
}

func (*FieldMatchNoMatch) isFieldMatchDescription() {}

// FieldMatchDirect branch of FieldMatchDescription
type FieldMatchDirect struct{}

func (d *FieldMatchDirect) String() string {
	return "same type"
}

func (*FieldMatchDirect) isFieldMatchDescription() {}

// FieldMatchConversion branch of FieldMatchDescription
type FieldMatchConversion struct {
	// MethodPrimary метод на primary-типе возвращающий значение secondary-типа
	MethodPrimary string
	// PrimaryToSecondary функция в пакете primary типа конвертирующая его в secondary
	PrimaryToSecondary string
	// PrimaryFromSecondary функция в пакете primary типа возвращающая его значение из secondary
	PrimaryFromSecondary string
	// MethodPrimary метод на secondary-типе возвращающий значение primary-типа
	MethodSecondary string
	// SecondaryToPrimary функция в пакете secondary-типа конвертирующая его в primary
	SecondaryToPrimary string
	// SecondaryToPrimary функция в пакете secondary типа возвращающая его значение из primary
	SecondaryFromPrimary string
}

func (c *FieldMatchConversion) String() string {
	if c.MethodPrimary != "" {
		return fmt.Sprintf(
			"convert primary to secondary with method %s, convert back with function %s",
			c.MethodPrimary,
			c.PrimaryFromSecondary,
		)
	}

	if c.PrimaryToSecondary != "" {
		return fmt.Sprintf(
			"convert primary to secondary with function %s and back with %s",
			c.PrimaryToSecondary,
			c.PrimaryFromSecondary,
		)
	}

	if c.MethodSecondary != "" {
		return fmt.Sprintf(
			"convert secondary to primary with method %s, convert back with function %s",
			c.MethodSecondary,
			c.SecondaryFromPrimary,
		)
	}

	return fmt.Sprintf(
		"convert secondary to primary with function %s and back with %s",
		c.SecondaryToPrimary,
		c.SecondaryFromPrimary,
	)
}

func (*FieldMatchConversion) isFieldMatchDescription() {}

// FieldMatchEnum branch of FieldMatchDescription
type FieldMatchEnum struct {
	Primary   *enumDescription
	Secondary *enumDescription
}

func (e *FieldMatchEnum) String() string {
	var values []string
	for _, v := range e.Primary.values {
		values = append(values, v.Val().ExactString())
	}

	return fmt.Sprintf("enumeration with values %s", strings.Join(values, ", "))
}

func (*FieldMatchEnum) isFieldMatchDescription() {}

// FieldMatchCastable branch of FieldMatchDescription
type FieldMatchCastable struct{}

func (a *FieldMatchCastable) String() string {
	return "assignable types"
}

func (*FieldMatchCastable) isFieldMatchDescription() {}

// FieldMatchSlice branch of FieldMatchDescription
type FieldMatchSlice struct {
	Elem FieldMatchDescription
}

func (s *FieldMatchSlice) String() string {
	return fmt.Sprintf("slice match where value is %s", s.Elem)
}

func (*FieldMatchSlice) isFieldMatchDescription() {}

// FieldMatchMap branch of FieldMatchDescription
type FieldMatchMap struct {
	Key  FieldMatchDescription
	Elem FieldMatchDescription
}

func (m *FieldMatchMap) String() string {
	return fmt.Sprintf("map match where key is %s and value is %s", m.Key, m.Elem)
}

func (*FieldMatchMap) isFieldMatchDescription() {}

var (
	_ FieldMatchDescription = &FieldMatchNoMatch{}
	_ FieldMatchDescription = &FieldMatchDirect{}
	_ FieldMatchDescription = &FieldMatchConversion{}
	_ FieldMatchDescription = &FieldMatchEnum{}
	_ FieldMatchDescription = &FieldMatchCastable{}
	_ FieldMatchDescription = &FieldMatchSlice{}
	_ FieldMatchDescription = &FieldMatchMap{}
)
