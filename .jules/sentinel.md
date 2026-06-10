## 2025-02-24 - [Update bcrypt hashing cost]
**Vulnerability:** Weak bcrypt hashing cost
**Learning:** Found instances of bcrypt using a low cost factor of `8` for password hashing, which makes password hashes vulnerable to brute force cracking attacks.
**Prevention:** Always use `bcrypt.DefaultCost` (which currently evaluates to 10 in Go) instead of hardcoding low-cost values to maintain secure, modern hashing standards.
