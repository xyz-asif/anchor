package cloudinary

import (
	"bytes"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/require"
)

func header(name string, size int64) *multipart.FileHeader {
	return &multipart.FileHeader{Filename: name, Size: size}
}

func TestValidationMessages(t *testing.T) {
	require.EqualError(t, ValidateImageFile(header("photo.png", 12*1024*1024+512*1024)), "This image is 12.5 MB. The maximum is 10 MB")
	require.EqualError(t, ValidateImageFile(header("photo.png", 0)), "This image is empty")
	require.EqualError(t, ValidateImageFile(header("photo.bmp", 10)), ".bmp isn't a supported image type. Supported types: jpg, jpeg, png, gif, webp")
	require.EqualError(t, ValidateFile(header("setup.exe", 10)),
		".exe files can't be uploaded. Supported: images (jpg, jpeg, png, gif, webp), audio (mp3, wav, aac, m4a, ogg) and documents (pdf, docx, doc, epub, txt)")
	require.ErrorContains(t, ValidateFile(header("README", 10)), "Files without an extension can't be uploaded")
	require.NoError(t, ValidateFile(header("notes.PDF", 10)))
}

func TestCheckDocumentContent(t *testing.T) {
	ok := map[string][]byte{
		"a.pdf":  []byte("%PDF-1.7\n..."),
		"a.docx": []byte("PK\x03\x04rest"),
		"a.txt":  []byte("Plain notes, ünïcödé and 日本語"),
	}
	for name, content := range ok {
		require.NoError(t, CheckDocumentContent(bytes.NewReader(content), name), name)
	}
	bad := map[string][]byte{
		"a.pdf": []byte("MZ\x90\x00 an executable"),
		"b.txt": []byte("<!DOCTYPE html><script>alert(1)</script>"),
		"c.txt": {0x00, 0x01, 0x02},
		"d.doc": []byte("PK\x03\x04 a zip, not a .doc"),
	}
	for name, content := range bad {
		require.ErrorContains(t, CheckDocumentContent(bytes.NewReader(content), name), "don't match its", name)
	}

	// The reader is rewound, so the upload sends the whole file
	r := bytes.NewReader([]byte("%PDF-1.4 body"))
	require.NoError(t, CheckDocumentContent(r, "x.pdf"))
	require.EqualValues(t, 0, r.Size()-int64(r.Len()))
}
