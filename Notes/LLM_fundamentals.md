## What is an LLM?

It is a neural network trained on massive amounts of text data to predict the next token in a sequence - given all the previous tokens, what comes next? - 

- **Model weights** - billions of numerical parameters learned during training that encode that model's knowledge
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

## Context window 

It's what the system remembers; it's measured through tokens, if exes the context limit it possible that the LLM will hallucinate

## Syatem/user messages 

The difference between user and system messages (prompts): The system message is the rules of the session and the llm; the user prompt is the messages of the task to resolve at the moment

 ## Temperature

 LLM generate text by prdicting the next word (or rather, next token), each token is assigned a logit (numerical value); "Softmax function" that exists between zero and one, and the sum of all tokens' softmax probabilities is one.

 The **Temperature** parameter modifies the districbution; a lower temparature essentially makes those tokens with the highest probability more likely to be selected. It's used to balace coherence and creativity.

 A high temperature value can make model outputs seem more creative but it's more accurate to think of them as being less determined by the training data.

## Model selection

 LLM model selection means choosing the right large language model for a specific task based on the minimum required capability rather than automatically using the biggest or most expensive model.
 
 ### Core Meaning
 - Matching Need to Task: Using a lightweight or smaller model for simple jobs (like sorting text) and saving flagship frontier models for complex work (like multi-step reasoning).
 - Avoiding Defaults: Moving away from the habit of letting the most powerful (and costly) model handle every single request by default.
 - Cost-Performance Balance: Finding the cheapest model that successfully clears your quality and performance threshold.
 
 ### Key Evaluation Factors
 
 When comparing models, teams look at several important metrics:
 - Quality/Accuracy: Does the model give correct and relevant answers?
 - Speed & Latency: How fast does the model start and finish generating text?
 - Cost: What is the price per query or token?
 - Context Window: How much data or text can the model process at one time?
 - Licensing: Whether the model is closed API, open-weight, or open-source.
 
## Hallucinations
An LLM hallucination is when a Large Language Model generates text that is grammatically correct and sounds confident, but is factually incorrect, misleading, or completely fabricated.

### Why Hallucinations Happen
- Statistical prediction: Models predict the next likely word based on patterns, not truth or real-world facts.
- Training data limits: Inaccurate internet data, biases, or missing information can lead to errors.
- Knowledge cutoffs: Models do not know real-time events that occurred after their training ended.
- Vague prompts: Unclear questions cause the model to guess or fill in missing context poorly

# Conceptos

Structured output
JSON schema
Prompting

Context management

https://jc1175.medium.com/a-crash-course-in-llm-context-management-543d515339f3