function mapScoreToRating(score) {
  if (score >= 0.9) return "AAA";
  if (score >= 0.8) return "AA";
  if (score >= 0.7) return "A";
  if (score >= 0.6) return "BBB";
  if (score >= 0.5) return "BB";
  if (score >= 0.4) return "B";
  if (score >= 0.3) return "C";
  return "D";
}

module.exports = { mapScoreToRating };
