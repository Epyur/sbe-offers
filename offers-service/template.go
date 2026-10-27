package main

import _ "embed"

// standardTemplateDocx — корпоративный бланк Word (шапка с логотипом, футер с
// реквизитами). Копия бланка agent-service: документы экосистемы должны
// выглядеть одинаково.
//
//go:embed template/standard.docx
var standardTemplateDocx []byte
