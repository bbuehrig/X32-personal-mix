package web

import "embed"

// FS contains the browser UI assets.
//
//go:embed index.html styles.css app.js sw.js manifest.json wake-ios.mp4 wake-ios.webm favicon.svg favicon-16.png favicon-32.png apple-touch-icon.png apple-touch-icon-v33.png icon-192.png icon-192-v33.png icon-512.png icon-512-v33.png icons
var FS embed.FS
