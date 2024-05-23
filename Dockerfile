# syntax = edrevo/dockerfile-plus
INCLUDE+ Dockerfile.include.start

ENV ENV=prod

INCLUDE+ Dockerfile.include.end

# Run the application
CMD ["./portal"]