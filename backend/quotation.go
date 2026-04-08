package backend

import (
	"context"
	"embed"
	"fmt"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/checkbox"
	"github.com/johnfercher/maroto/v2/pkg/components/code"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/signature"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed assets/*
var backAssets embed.FS

type Quotation struct {
	ctx context.Context
}

func Startup(q *Quotation, ctx context.Context) {
	q.ctx = ctx
}

func (q *Quotation) Generate() {
	imgByte, err := backAssets.ReadFile("assets/cats.png")

	if err != nil {
		fmt.Println(err)
	}

	filePath, err := runtime.SaveFileDialog(q.ctx, runtime.SaveDialogOptions{
		Title:           "Save Quotation",
		DefaultFilename: "quotation.pdf",
		Filters:         []runtime.FileFilter{{DisplayName: "PDF Files (*.pdf)", Pattern: "*.pdf"}},
	})

	m := q.createQuotation(imgByte)

	document, err := m.Generate()

	if err != nil {
		fmt.Println(err)
	}

	err = document.Save(filePath)

	if err != nil {
		fmt.Println(err)
	}
}

func (q *Quotation) createQuotation(img []byte) core.Maroto {
	m := maroto.New()

	m.AddRow(
		40,
		code.NewBarCol(4, "barcode", props.Barcode{Percent: 80, Center: true}),
		code.NewMatrixCol(4, "matrixcode", props.Rect{Percent: 80, Center: true}),
		code.NewQrCol(4, "qrcode", props.Rect{Percent: 75, Center: true}),
	)

	m.AddRow(10, col.New(12))

	m.AddRow(
		20,
		image.NewFromBytesCol(4, img, extension.Png, props.Rect{Percent: 100, Center: true}),
		signature.NewCol(4, "signature"),
		text.NewCol(4, "text"),
	)

	m.AddRow(10, col.New(12))

	m.AddRow(20, checkbox.NewCol(12, "agree"))

	m.AddRow(20, col.New(12))

	return m
}
