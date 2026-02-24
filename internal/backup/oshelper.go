package backup

import "os"

// osReadFile / osWriteFile - 避免在模板函数中重复 import os
func osReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func osWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
