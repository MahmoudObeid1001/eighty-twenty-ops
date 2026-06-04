import re

with open("web/pages/placement_result.html", "r") as f:
    content = f.read()

# Extract the sections
# 1. SCREENSHOT UPLOAD
upload_match = re.search(r'(<hr class="divider" style="margin-top:20px;">\s*<!-- SCREENSHOT UPLOAD -->.*?</section>)', content, re.DOTALL)
upload_block = upload_match.group(1)

# 2. BUNDLE PICKER
bundle_match = re.search(r'(<hr class="divider" style="margin-top:20px;">\s*<!-- BUNDLE PICKER -->.*?</section>)', content, re.DOTALL)
bundle_block = bundle_match.group(1)

# 3. PAYMENT DETAILS (up to price + CTA)
pay_match = re.search(r'(<hr class="divider" style="margin-top:20px;">\s*<section class="pay-section">.*?)(?=<!-- price \+ CTA -->)', content, re.DOTALL)
pay_block = pay_match.group(1)

# 4. PRICE AND CTA
cta_match = re.search(r'(<!-- price \+ CTA -->.*?</section>)', content, re.DOTALL)
cta_block = cta_match.group(1)

# Remove them all from the content (from the first one to the end of cta)
start_idx = content.find(upload_block)
end_idx = content.find(cta_block) + len(cta_block)

new_content = content[:start_idx] + \
    bundle_block + "\n" + \
    pay_block + "\n" + \
    "  </section>\n" + \
    upload_block + "\n" + \
    '  <section class="section" style="padding-top:10px;">\n    ' + cta_block + "\n" + \
    content[end_idx:]

with open("web/pages/placement_result.html", "w") as f:
    f.write(new_content)
