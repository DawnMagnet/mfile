function answer(n) {
  return n * 2;
}

const values = [1, 2, 3];
const total = values.map(answer).reduce((a, b) => a + b, 0);
console.log(`total ${total}`);
