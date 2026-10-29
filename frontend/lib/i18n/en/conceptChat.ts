// The chat about one concept (ConceptChat.tsx).
const en = {
  starters: ["How would I build it, step by step?", "What are the inputs and outputs? Show an example.", "Which public data or APIs does it rely on?", "How do I get the first 100 users?"],
  intro: (price: number) => `Ask anything about building or launching it. Each answer costs ${price} credits; the thread is kept with the concept.`,
  thinking: "Thinking…",
  placeholder: "How would I find bots to audit with public data?",
  ask: "Ask",
  cr: "cr",
};

export default en;
