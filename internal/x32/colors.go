package x32

// ColorCSS returns a CSS color for an X32 channel color index (0–15).
// 0 OFF, 1–7 RD GN YE BL MG CY WH, 8–15 inverted variants.
func ColorCSS(id int) string {
	if id < 0 || id >= len(colorCSS) {
		return colorCSS[0]
	}
	return colorCSS[id]
}

var colorCSS = []string{
	"#2a3340", // OFF
	"#c23b3b", // RD
	"#2f9e5a", // GN
	"#c9a83a", // YE
	"#3b6fc2", // BL
	"#a83b9e", // MG
	"#2f9e9e", // CY
	"#c8d0da", // WH
	"#1e2430", // OFFi
	"#e85a5a", // RDi
	"#3ecf8e", // GNi
	"#f0d35a", // YEi
	"#5a9cf0", // BLi
	"#d45ac8", // MGi
	"#5ad4d4", // CYi
	"#eef2f7", // WHi
}
