// Package ocr 提供图像文字识别能力。
// 用于识别职位表截图、毕业证/学位证等图片中的文字信息。
package ocr

// Recognizer OCR 识别器接口，便于替换不同 OCR 服务实现。
type Recognizer interface {
	// Recognize 识别图片中的文字，image 为图片二进制内容。
	Recognize(image []byte) (string, error)
}
