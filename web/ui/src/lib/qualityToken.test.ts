import { describe, expect, it } from "vitest";
import { hasQualityToken, setQualityToken } from "./qualityToken";

describe("setQualityToken", () => {
  const withQ = "{Series Title} - S{season:00}E{episode:00} - {Episode Title} {Quality Full}";
  const without = "{Series Title} - S{season:00}E{episode:00} - {Episode Title}";

  it("removes the token and the space before it", () => {
    expect(setQualityToken(withQ, false)).toBe(without);
  });
  it("removes a token in the middle", () => {
    expect(setQualityToken("{Series Title} {Quality Full} - {Episode Title}", false)).toBe("{Series Title} - {Episode Title}");
  });
  it("appends the token once", () => {
    expect(setQualityToken(without, true)).toBe(withQ);
    expect(setQualityToken(withQ, true)).toBe(withQ);
  });
  it("leaves an empty format empty", () => {
    expect(setQualityToken("", true)).toBe("");
  });
  it("detects the token", () => {
    expect(hasQualityToken(withQ)).toBe(true);
    expect(hasQualityToken(without)).toBe(false);
  });
});
