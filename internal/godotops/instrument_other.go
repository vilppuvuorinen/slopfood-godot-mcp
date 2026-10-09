//go:build !gdcover

package godotops

func instrumentOperations(content string) (string, error) { return content, nil }

func instrumentValidateScript(content string) (string, error) { return content, nil }
