ExUnit.start()

# The outbox consumer, listener, endpoint and Oban are deliberately NOT started
# in :test (see AtlasOrchestrator.Application), so tests own the Ecto sandbox
# without unrelated processes competing for connections. The live path — Go
# writes, the consumer reacts — is verified by running the service.
