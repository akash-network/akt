package logo

import (
	"io"

	"github.com/gookit/color"
)

func Write(iow io.Writer) (int, error) {
	var wrote int
	{
		n, err := io.WriteString(iow, emblem)
		wrote += n
		if err != nil {
			return n, err
		}
	}

	{
		n, err := io.WriteString(iow, color.Bold.Sprint(wordmark))
		wrote += n
		if err != nil {
			return n, err
		}
	}
	return wrote, nil
}
