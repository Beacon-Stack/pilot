// The "Include quality in file names" switch on the media management page
// edits the naming formats directly: on appends {Quality Full}, off removes
// it. The formats stay the single source of truth, so a hand-edited format
// keeps working and the switch simply reflects it.

export const QUALITY_TOKEN = "{Quality Full}";

export function hasQualityToken(format: string): boolean {
  return format.includes(QUALITY_TOKEN);
}

/** Adds {Quality Full} to the end of format, or removes every occurrence. */
export function setQualityToken(format: string, include: boolean): string {
  if (include) {
    return hasQualityToken(format) || format.trim() === "" ? format : `${format.trimEnd()} ${QUALITY_TOKEN}`;
  }
  return format
    .split(QUALITY_TOKEN)
    .join("")
    .replace(/\s{2,}/g, " ")
    .replace(/\s+([.)\]])/g, "$1")
    .trim();
}
