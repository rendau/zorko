package model

// Result — итог похода на сайт за картой: файлы в архиве BundleId или код причины неудачи
type Result struct {
	BundleId int64
	JsBlob   string
	MapBlob  string
	MapUrl   string
	Code     string // пусто — удача
}
