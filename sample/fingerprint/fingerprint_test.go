package fingerprint

import (
	"strings"
	"testing"

	"github.com/mfonda/simhash"
)

func TestFingerprint(t *testing.T) {
	const s1 = `
<html>
<head>
</head>
<body>
<p>Hello, World</p>
</body>
</html>
`
	const s2 = `
<html>
<head>
</head>
<body>
<p>你好，世界</p>
<p>维基 百科</p>
</body>
</html>
`
	const s3 = `
<html>
<head>
</head>
<body>
<p>Hello, World</p>
<p>你好，世界</p>
</body>
</html>
`
	f1 := Compute(strings.NewReader(s1), 4096, 2)
	f2 := Compute(strings.NewReader(s2), 4096, 2)
	f3 := Compute(strings.NewReader(s3), 4096, 2)
	if d := simhash.Compare(f1, f2); d > 3 {
		t.Errorf("distance should <= 3, actual: %d\n", d)
	}
	if d := simhash.Compare(f2, f3); d > 3 {
		t.Errorf("distance should <= 3, actual: %d\n", d)
	}
}
