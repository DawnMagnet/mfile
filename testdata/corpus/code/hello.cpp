#include <iostream>
#include <string>
#include <vector>

class Sample {
public:
    explicit Sample(std::string name) : name_(std::move(name)) {}
    void greet() const { std::cout << "hello " << name_ << std::endl; }
private:
    std::string name_;
};

int main() {
    std::vector<Sample> items;
    items.emplace_back("qa");
    for (const auto& item : items) {
        item.greet();
    }
    return 0;
}
