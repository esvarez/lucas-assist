## What is an LLM?

Is a neural network trained on massive amounts of text data to predict the next token sequence - given all the previous tokens, what comes next? - 

- **Model weights** - billions of numerical parameters learned during trainging that encode that model's knowledge
- **Architecture code** - the neural network structure (typically a transformer) that runs weights to produce output 

## Tokens

The LLM's don't process raw text - they work wich tokens, they are sub-word units from fixed vocabulary

For example, the sentence "Tokenization is fascinating!" might break down into tokens like:

```
["Token", "ization", " is", " fascinating", "!"]
```

Each token maps to a number (an ID in the model's vocabulary), and the model operates entirely on these numbers — not on text. When the model produces output, it generates token IDs that are then decoded back into text.

```
[4421, 2860, 382, 33733, 0]
```

- Pricing is typically per-token (input tokens + output tokens)
- Context windows are measured in tokens (not words or characters)
- Longer prompts use more tokens, cost more, and leave less room for the model's response
# Conceptos

Context window
System/user messages
Temperature
Model selection
Structured output
JSON schema
Prompting
Hallucinations
Context management