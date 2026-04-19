#!/usr/bin/env python3
import urllib.request
import json

TOKEN = "ca9fe19098e0a8069692d7b9c9d4443c6680be63db57db4f33ab56702a29bf583ed94bcfedec491ae0ccde201db37670dc33c494f029bc1435dcd30f93569849d87ef93fe6a92762142b631aff452cb1b106648e555224307aa74e98ac71fd76260a4202d5e4ea17e43937981cf6fde19a5f4f0e940e05084d3771137c876c6e"
ENDPOINT = "https://back-strapi.kawasaki.com.pe/graphql"
DOCUMENT_ID = "fbhykr7sbrnn3040pwfegm2k"

TARGET_SLUG = "mejores-tipos-moto-rutas-full-day"
OLD = "manera electrónica/15 cm,"
NEW = "manera electrónica/15 cm(Como estaaaa),"

# ── 1. FETCH ──────────────────────────────────────────────────
fetch_query = json.dumps(
    {
        "query": "{ blog { blog_list { verBlog slug title date fullDescription } } }"
    }
)
req = urllib.request.Request(
    ENDPOINT,
    data=fetch_query.encode("utf-8"),
    headers={
        "Authorization": f"Bearer {TOKEN}",
        "Content-Type": "application/json",
        "Origin": "https://www.kawasaki.com.pe",
    },
)
with urllib.request.urlopen(req) as r:
    data = json.loads(r.read())

blog_list = data["data"]["blog"]["blog_list"]

# ── 2. PATCH only TARGET_SLUG ─────────────────────────────────
patches = 0
for post in blog_list:
    if post["slug"] != TARGET_SLUG:
        continue
    for block in post.get("fullDescription", []):
        for child in block.get("children", []):
            if child.get("type") == "text" and OLD in child.get("text", ""):
                child["text"] = child["text"].replace(OLD, NEW)
                patches += 1
                print(f"✅ Patched in post: {post['slug']}")

if patches == 0:
    print("❌ Text not found — nothing to update")
    exit(1)

print(f"\n📝 Total patches: {patches}")

# ── 3. SEND ───────────────────────────────────────────────────
mutation = json.dumps(
    {
        "query": """
        mutation UpdateBlog($data: BlogInput!) {
            updateBlog(data: $data) {
                documentId
            }
        }
    """,
        "variables": {
            "data": {
                "blog_list": [
                    {
                        "verBlog": post["verBlog"],
                        "slug": post["slug"],
                        "title": post["title"],
                        "date": post["date"],
                        "fullDescription": post["fullDescription"],
                    }
                    for post in blog_list
                ]
            }
        },
    }
)

req2 = urllib.request.Request(
    ENDPOINT,
    data=mutation.encode("utf-8"),
    headers={
        "Authorization": f"Bearer {TOKEN}",
        "Content-Type": "application/json",
        "Origin": "https://www.kawasaki.com.pe",
    },
)

try:
    with urllib.request.urlopen(req2) as r:
        result = json.loads(r.read())
    if result.get("data"):
        print(
            f"\n🎉 Updated! documentId: {result['data']['updateBlog']['documentId']}"
        )
    else:
        print(f"\n❌ GraphQL error: {json.dumps(result, indent=2)}")
except urllib.error.HTTPError as e:
    body = e.read().decode("utf-8")
    print(f"\n❌ HTTP {e.code}: {body}")
