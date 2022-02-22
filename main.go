package main

import (
	"os"

	"awesome-converter/internal/app"
	"github.com/alecthomas/kong"
	"github.com/sirkon/message"
	"github.com/willabides/kongplete"

	"gitlab.stageoffice.ru/UCS-COMMON/errors"
)

func main() {
	var cli cliArgs
	cli.Generate.Primary.needLocal = true
	parser := kong.Must(
		&cli,
		kong.Name(app.Name),
		kong.Description(
			`Code generator for conversions between Go structures. Generated code is to be placed where the primary structure is defined`,
		),
		kong.ConfigureHelp(kong.HelpOptions{
			Summary: true,
			Compact: true,
		}),
		kong.UsageOnError(),
	)

	kongplete.Complete(
		parser,
		kongplete.WithPredictor("local-struct-path", &cli.Generate.Primary),
		kongplete.WithPredictor("free-struct-path", &cli.Generate.Secondary),
	)

	ctx, err := parser.Parse(os.Args[1:])
	if err != nil {
		parser.FatalIfErrorf(err)
	}

	if err := ctx.Run(&RunContext{
		args: &cli,
	}); err != nil {
		message.Fatal(errors.Wrap(err, "run command"))
	}
}

// import (
// 	"fmt"
// 	"fyne.io/fyne/v2"
// 	"fyne.io/fyne/v2/app"
// 	"fyne.io/fyne/v2/container"
// 	"fyne.io/fyne/v2/widget"
// )
//
// type (
// 	fieldPair struct {
// 		primary   string
// 		secondary string
// 	}
// )
//
// var (
// 	// mappedFields список полей для которых было установлено взаимо-однозначное соответствие
// 	mappedFields = []string{
// 		"id",
// 		"region_id",
// 		"tenant_id",
// 	}
// )
//
// func main() {
// 	a := app.New()
// 	w := a.NewWindow("Hello")
//
// 	lt := container.NewVBox()
// 	lt.Add(widget.NewLabel("Конвертируемые поля"))
// 	labelMapping := map[string]fyne.CanvasObject{}
// 	for _, n := range mappedFields {
// 		var w fyne.CanvasObject
// 		wgt := widget.NewButton(n, func() {
// 			lt.Remove(w)
// 		})
// 		w = wgt
// 		labelMapping[n] = wgt
// 		lt.Add(wgt)
// 	}
//
// 	w.SetContent(container.NewVBox(
// 		container.NewHBox(
// 			lt,
// 			container.NewVBox(
// 				widget.NewTextGrid(),
// 				widget.NewLabel("Свободные поля primary"),
// 				widget.NewButton("domains", nil),
// 			),
// 			container.NewVBox(
// 				widget.NewLabel("Свободные поля secondary"),
// 				widget.NewButton("extension", nil),
// 			),
// 		),
// 		widget.NewButton("Hi!", func() {
// 			fmt.Println("clicked")
// 		}),
// 	))
//
// 	w.ShowAndRun()
// }
//
// func listToMapping(src []string) []fieldPair {
// 	res := make([]fieldPair, 0, len(src))
// 	for _, x := range src {
// 		res = append(res, fieldPair{
// 			primary:   x,
// 			secondary: x,
// 		})
// 	}
//
// 	return res
// }
