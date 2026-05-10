package openaiapi

import gen "github.com/vex9z7/llama.cpp-stack/gateway/internal/openaiapi/generated"

type ChatCompletion = gen.ChatCompletion
type Completion = gen.Completion
type CompletionTokensDetails = gen.CompletionTokensDetails
type CompletionUsage = gen.CompletionUsage
type EasyInputMessage = gen.EasyInputMessage
type EasyInputMessageContent = gen.EasyInputMessageContent
type EasyInputMessageRole = gen.EasyInputMessageRole
type EasyInputMessageType = gen.EasyInputMessageType
type EmbeddingResponse = gen.EmbeddingResponse
type EmbeddingUsage = gen.EmbeddingUsage
type ErrorBody = gen.ErrorBody
type ErrorObject = gen.ErrorObject
type InputMessageContent = gen.InputMessageContent
type InputMessageContentType = gen.InputMessageContentType
type Model = gen.Model
type ModelList = gen.ModelList
type ModelMeta = gen.ModelMeta
type ModelRequest = gen.ModelRequest
type PromptTokensDetails = gen.PromptTokensDetails
type Response = gen.Response
type ResponseCreateRequest = gen.ResponseCreateRequest
type ResponseFunctionCall = gen.ResponseFunctionCall
type ResponseFunctionCallOutput = gen.ResponseFunctionCallOutput
type ResponseInput = gen.ResponseInput
type ResponseInputItem = gen.ResponseInputItem
type ResponseInputTokensDetails = gen.ResponseInputTokensDetails
type ResponseOutputTokensDetails = gen.ResponseOutputTokensDetails
type ResponseUsage = gen.ResponseUsage

const (
	EasyInputMessageRoleAssistant EasyInputMessageRole = gen.Assistant
	EasyInputMessageRoleDeveloper EasyInputMessageRole = gen.Developer
	EasyInputMessageRoleSystem    EasyInputMessageRole = gen.System
	EasyInputMessageRoleUser      EasyInputMessageRole = gen.User

	EasyInputMessageTypeMessage EasyInputMessageType = gen.Message

	InputMessageContentTypeInputText  InputMessageContentType = gen.InputText
	InputMessageContentTypeOutputText InputMessageContentType = gen.OutputText
)
