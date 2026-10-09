//go:build gdcover

package godotops

var instrumentBridge func(string) (string, error)

var instrumentValidate func(string) (string, error)

func instrumentOperations(content string) (string, error) {
	if instrumentBridge == nil {
		return content, nil
	}
	return instrumentBridge(content)
}

func instrumentValidateScript(content string) (string, error) {
	if instrumentValidate == nil {
		return content, nil
	}
	return instrumentValidate(content)
}
