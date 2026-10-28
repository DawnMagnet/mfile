interface Sample {
  id: number;
  name: string;
}

function greet(s: Sample): string {
  return `${s.id}:${s.name}`;
}

const item: Sample = { id: 1, name: "qa" };
console.log(greet(item));
