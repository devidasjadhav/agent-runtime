package tool

func ObjectSchema(required []string, properties map[string]ToolPropertySchema) ToolSchema {
	return ToolSchema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

func StringProperty(description string) ToolPropertySchema {
	return ToolPropertySchema{Type: "string", Description: description}
}

func StringPropertyDefault(description string, defaultValue any) ToolPropertySchema {
	return ToolPropertySchema{Type: "string", Description: description, Default: defaultValue}
}

func IntegerProperty(description string, defaultValue any) ToolPropertySchema {
	return ToolPropertySchema{Type: "integer", Description: description, Default: defaultValue}
}

func BooleanProperty(description string, defaultValue any) ToolPropertySchema {
	return ToolPropertySchema{Type: "boolean", Description: description, Default: defaultValue}
}

func StringEnumProperty(description string, values []string, defaultValue any) ToolPropertySchema {
	return ToolPropertySchema{
		Type:        "string",
		Description: description,
		Enum:        values,
		Default:     defaultValue,
	}
}
