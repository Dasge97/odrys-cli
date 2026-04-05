function extractText(payload) {
  const text = payload?.choices?.[0]?.message?.content
  if (typeof text === "string") return text
  if (Array.isArray(text)) {
    return text
      .map((item) => item?.text ?? "")
      .join("")
      .trim()
  }
  throw new Error("La respuesta del proveedor no contiene texto util")
}

function parseJson(text) {
  try {
    return JSON.parse(text)
  } catch {
    const start = text.indexOf("{")
    const end = text.lastIndexOf("}")
    if (start !== -1 && end !== -1 && end > start) {
      return JSON.parse(text.slice(start, end + 1))
    }
    throw new Error("No se pudo parsear JSON de la respuesta del modelo")
  }
}

export class OpenAICompatibleProvider {
  constructor(config) {
    this.config = config
    this.baseUrl = process.env.ODYRS_BASE_URL || "https://api.openai.com/v1"
    this.apiKey = process.env.ODYRS_API_KEY || process.env.OPENAI_API_KEY
    this.model = config.model
  }

  async run(input) {
    if (!this.apiKey) {
      throw new Error("Falta ODYRS_API_KEY u OPENAI_API_KEY para usar openai-compatible")
    }

    const response = await fetch(`${this.baseUrl}/chat/completions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${this.apiKey}`
      },
      body: JSON.stringify({
        model: this.model,
        temperature: 0.1,
        response_format: { type: "json_object" },
        messages: [
          {
            role: "system",
            content: input.systemPrompt
          },
          {
            role: "user",
            content: `Objetivo:\n${input.goal}\n\nTarea actual:\n${input.task}\n\nContexto:\n${input.context}\n\nFeedback previo:\n${JSON.stringify(input.feedback ?? null, null, 2)}\n\nResponde solo con JSON valido segun el contrato del agente ${input.agent}.`
          }
        ]
      })
    })

    if (!response.ok) {
      throw new Error(`Error del proveedor: ${response.status} ${await response.text()}`)
    }

    const payload = await response.json()
    return parseJson(extractText(payload))
  }
}
