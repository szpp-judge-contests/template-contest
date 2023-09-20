#include "testlib.h"

int main() {
    registerValidation(); 

    inf.readInt(0, 10); // A
    inf.readSpace();
    inf.readInt(0, 10); // B
    inf.readChar('\n');
    inf.readEof();

    return 0;
}
