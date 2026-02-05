require "json"
require_relative "helpers/format"

module Fixtures
  class Processor
    def run(value)
      payload = normalize(value)
      JSON.dump(payload)
    end

    def normalize(value)
      value.strip.downcase
    end
  end
end
