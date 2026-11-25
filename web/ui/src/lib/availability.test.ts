import { describe, expect, it } from "vitest";
import { asAvailability, availabilityLabel } from "./availability";

describe("asAvailability", () => {
  it("accepts the states it knows and nothing else", () => {
    expect(asAvailability("available")).toBe("available");
    expect(asAvailability("teleported")).toBeUndefined();
    expect(asAvailability(undefined)).toBeUndefined();
  });
});

describe("availabilityLabel", () => {
  it("shows the percentage while downloading", () => {
    expect(availabilityLabel("downloading", 0.256)).toBe("Downloading 25%");
    expect(availabilityLabel("downloading", 0)).toBe("Downloading");
    expect(availabilityLabel("available")).toBe("Available");
  });
});
