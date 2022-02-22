package main

import (
	"os"
	"strings"

	"awesome-converter/internal/generator"
	"github.com/sirkon/jsonexec"

	"gitlab.stageoffice.ru/UCS-COMMON/errors"
	"gitlab.stageoffice.ru/UCS-COMMON/matiss/v2"
)

// GenerateCommand команда генерации преобразований.
type GenerateCommand struct {
	Primary       structPath `arg:"" help:"Primary structure to generate conversions in its package. Must look like <rel-path>:<name>." predictor:"local-struct-path"`
	Secondary     structPath `arg:"" help:"Secondary structure to generate conversions to and from the primary one. Must look like <pkg-path>:<name>." predictor:"free-struct-path"`
	PrimaryMethod string     `short:"m" help:"MethodPrimary name for the primary -> secondary conversion. Free function will be generated instead if not set."`
}

// Run запуск генерации
func (c *GenerateCommand) Run(rctx *RunContext) error {
	var listInfo struct {
		Dir  string
		Path string
	}
	if err := jsonexec.Run(&listInfo, "go", "list", "-m", "--json"); err != nil {
		return errors.Wrap(err, "retrieve current module information")
	}

	g, err := generator.New(
		undottedPrefix(c.Primary.pkgPath, listInfo.Path),
		c.Primary.name,
		undottedPrefix(c.Secondary.pkgPath, listInfo.Path),
		c.Secondary.name,
		c.PrimaryMethod,
	)
	if err != nil {
		return errors.Wrap(err, "setup generator")
	}

	prj, err := matiss.UpdateProject()
	if err != nil {
		return errors.Wrap(err, "setup matiss for the current project")
	}

	if err := g.Generate(prj); err != nil {
		return errors.Wrap(err, "generate source code")
	}

	dir := matiss.Directory(".")
	if err := prj.Render(dir); err != nil {
		return errors.Wrap(err, "render generated source code")
	}

	return nil
}

func undottedPrefix(pkg, modPkg string) string {
	if strings.HasPrefix(pkg, "."+string(os.PathSeparator)) {
		return strings.Replace(pkg, "."+string(os.PathSeparator), modPkg+string(os.PathSeparator), 1)
	}

	return pkg
}
