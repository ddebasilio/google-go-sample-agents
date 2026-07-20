import { GoogleGenAI } from "@google/genai";

const client = new GoogleGenAI({});

const interaction = await client.interactions.create({
  agent: "antigravity-preview-05-2026",
  input:
    "Write a Python script that generates the first 20 Fibonacci numbers and saves them to fibonacci.txt. Then read the file and print its contents.",
  environment: "remote",
});

console.log(`Interaction ID: ${interaction.id}`);
console.log(`Environment ID: ${interaction.environment_id}`);

console.log(`Output: ${interaction.output_text}`);

const interaction2 = await client.interactions.create({
  agent: "antigravity-preview-05-2026",
  input:
    "what's the sum of the individual numbers of the last calculated number?",
  environment: interaction.environment_id,
});

console.log(`Output: ${interaction2.output_text}`);
