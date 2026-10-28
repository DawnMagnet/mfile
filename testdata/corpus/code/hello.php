<?php
class Sample {
    public function __construct(private string $name) {}
    public function greet(): string {
        return "hello " . $this->name;
    }
}
echo (new Sample("qa"))->greet(), PHP_EOL;
