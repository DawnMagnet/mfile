#include <stdio.h>
#include <string.h>

struct sample {
    int id;
    char name[32];
};

static int answer(const struct sample *s) {
    return s->id + (int)strlen(s->name);
}

int main(void) {
    struct sample s;
    s.id = 7;
    strcpy(s.name, "qa");
    printf("%d\n", answer(&s));
    return 0;
}
