class Sample:
    def __init__(self, name):
        self.name = name

    def greet(self):
        return "hello " + self.name


def main():
    print(Sample("qa").greet())


if __name__ == "__main__":
    main()
