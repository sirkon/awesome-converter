package main

import (
	"testing"

	"github.com/posener/complete"
)

func Test_structPath_Predict(t *testing.T) {
	p := &structPath{
		needLocal: false,
		pkgPath:   "",
		name:      "",
	}
	ps := p.Predict(
		complete.Args{
			Last: "gitlab.stageoffice.ru/UCS-COMMON/schemagen-go/v41/",
		},
	)
	t.Log(ps)
}
