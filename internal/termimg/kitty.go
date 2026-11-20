package termimg

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strconv"
	"strings"
)

// EncodeKitty transmits img as image id (PNG) without displaying it.
func EncodeKitty(img image.Image, id int) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	data := base64.StdEncoding.EncodeToString(buf.Bytes())

	var sb strings.Builder
	const chunk = 4096
	for i := 0; i < len(data); i += chunk {
		end := min(i+chunk, len(data))
		more := 0
		if end < len(data) {
			more = 1
		}
		sb.WriteString("\x1b_G")
		if i == 0 {
			// a=t transmit only, f=100 PNG, q=2 never answer (answers would
			// arrive as keypresses).
			sb.WriteString("a=t,f=100,q=2,i=")
			sb.WriteString(strconv.Itoa(id))
			sb.WriteByte(',')
		}
		sb.WriteString("m=")
		sb.WriteString(strconv.Itoa(more))
		sb.WriteByte(';')
		sb.WriteString(data[i:end])
		sb.WriteString("\x1b\\")
	}
	return sb.String(), nil
}

// KittyPlace shows transmitted image id at the cursor over cols×rows cells
// without moving the cursor.
func KittyPlace(id, cols, rows int) string {
	return "\x1b_Ga=p,q=2,C=1,i=" + strconv.Itoa(id) + ",p=1,c=" + strconv.Itoa(cols) + ",r=" + strconv.Itoa(rows) + "\x1b\\"
}

// KittyDelete removes the placements of image id (the data stays cached).
func KittyDelete(id int) string {
	return "\x1b_Ga=d,d=i,q=2,i=" + strconv.Itoa(id) + "\x1b\\"
}
