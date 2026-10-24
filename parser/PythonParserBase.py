import sys
from enum import Enum
from typing import TextIO
from antlr4 import Parser, Token, TokenStream

class PythonVersion(Enum):
    Autodetect = 0
    Python2 = 2
    Python3 = 3

class PythonParserBase(Parser):
    def __init__(self, input_stream: TokenStream, output: TextIO = sys.stdout):
        super().__init__(input_stream, output)
        self.__version = PythonVersion.Autodetect

    @property
    def version(self) -> PythonVersion:
        return self.__version

    @version.setter
    def version(self, version: PythonVersion | int):
        if isinstance(version, PythonVersion):
            self.__version = version
        else:
            self.__version = PythonVersion(version)

    def CheckVersion(self, version: int) -> bool:
        return self.__version == PythonVersion.Autodetect or version == self.__version.value

    def SetVersion(self, required_version: int) -> None:
        self.__version = PythonVersion(required_version)
    def IsSoftKeyword(self, word: str) -> bool:
        # Words such as `until` are keywords only inside a temporal formula,
        # so they lex as NAME and the grammar checks the text.
        return self._input.LT(1).text == word

    def ReturnsPossibly(self) -> bool:
        # `return possibly (Q)` also reads as a call to a function named
        # possibly, so a return whose expression uses `possibly` followed by
        # an operand is parsed as a formula. Outside an assertion, the
        # formula falls back to the plain return it would have been.
        if self._input.LA(1) != self.RETURN:
            return False
        operand_starts = (self.NAME, self.OPEN_PAREN, self.NOT, self.ALWAYS, self.EVENTUALLY, self.EXISTS)
        i = 2
        while self._input.LA(i) not in (self.LINE_BREAK, Token.EOF):
            t = self._input.LT(i)
            if t.type == self.NAME and t.text == "possibly" and self._input.LA(i + 1) in operand_starts:
                return True
            i += 1
        return False
