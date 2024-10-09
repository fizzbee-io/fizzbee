FROM chainguard/bazel:latest

WORKDIR /app

COPY . .

RUN bazel build parser/parser_bin
RUN bazel build --platforms=//:linux_x86 //:fizzbee

ENTRYPOINT ["/app/fizz"]
