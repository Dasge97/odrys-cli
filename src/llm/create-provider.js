import { MockProvider } from "./mock-provider.js"
import { OpenAICompatibleProvider } from "./openai-compatible-provider.js"

export function createProvider(config) {
  if (config.name === "mock") {
    return new MockProvider(config)
  }

  if (config.name === "openai-compatible") {
    return new OpenAICompatibleProvider(config)
  }

  throw new Error(`Proveedor no soportado: ${config.name}`)
}
